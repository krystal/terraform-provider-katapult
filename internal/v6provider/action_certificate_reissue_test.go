package v6provider

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/next/core"
	"github.com/stretchr/testify/require"
)

// TestAccKatapultCertificateReissue_self_signed runs the reissue action from a
// terraform_data action_trigger. Because the test uses the muxed provider
// factory, a passing run also proves the mux server advertises the action
// schema.
func TestAccKatapultCertificateReissue_self_signed(t *testing.T) {
	tt := newTestTools(t)
	name := testAccCertificateHostname(tt)
	res := "katapult_self_signed_certificate.web"

	config := undent.Stringf(`
		resource "katapult_self_signed_certificate" "web" {
		  name = "%s"
		}

		action "katapult_certificate_reissue" "web" {
		  config {
		    certificate_id = katapult_self_signed_certificate.web.id
		  }
		}

		resource "terraform_data" "reissue" {
		  lifecycle {
		    action_trigger {
		      events  = [after_create]
		      actions = [action.katapult_certificate_reissue.web]
		    }
		  }
		}`,
		name,
	)

	resource.ParallelTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultCertificateDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: config,
				// The action re-issues the certificate after the resource
				// state is written, so only the API state is checked here; the
				// refreshed outputs are compared after the next plan.
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultCertificateIssued(tt, res),
					resource.TestCheckResourceAttr(res, "state", "issued"),
				),
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultCertificateAttrs(tt, res),
					resource.TestCheckResourceAttr(res, "state", "issued"),
				),
			},
		},
	})
}

func TestCertificateReissueActionValidateConfig(t *testing.T) {
	t.Parallel()

	for value, wantErr := range map[string]bool{
		"10m": false, "90s": false, "0s": true, "-1m": true, "soon": true,
	} {
		a := &CertificateReissueAction{}
		req := certificateReissueInvokeRequest(t, a, CertificateReissueActionModel{
			CertificateID: types.StringValue("cert_x"),
			Timeout:       types.StringValue(value),
		})
		resp := &action.ValidateConfigResponse{}
		a.ValidateConfig(context.Background(),
			action.ValidateConfigRequest(req), resp)
		require.Equalf(t, wantErr, resp.Diagnostics.HasError(), "timeout %q", value)
	}
}

func TestCertificateReissueActionInvoke(t *testing.T) {
	tests := map[string]struct {
		timeout       types.String
		issueStatus   int
		issueBody     string
		taskStatuses  []string
		wantErr       []string
		wantMinPolls  int32
		wantZeroPolls bool
	}{
		"polls task to completion": {
			timeout:     types.StringNull(),
			issueStatus: http.StatusOK,
			taskStatuses: []string{
				string(core.TaskStatusEnumPending), "running", "completed",
			},
			wantMinPolls: 3,
		},
		"failed task": {
			timeout:      types.StringValue("1m"),
			issueStatus:  http.StatusOK,
			taskStatuses: []string{"failed"},
			wantErr:      []string{"Certificate Reissue Error", "task failed"},
			wantMinPolls: 1,
		},
		"honors timeout": {
			timeout:      types.StringValue("500ms"),
			issueStatus:  http.StatusOK,
			taskStatuses: []string{"running"},
			wantErr:      []string{"Certificate Reissue Error", "timeout"},
			wantMinPolls: 2,
		},
		"custom certificate": {
			timeout:     types.StringNull(),
			issueStatus: http.StatusUnprocessableEntity,
			issueBody: `{"error": {"code": "operation_not_supported",
				"description": "Custom certificates cannot be re-issued"}}`,
			wantErr: []string{
				"Certificate Reissue Not Supported",
				"cert_x cannot be re-issued",
				"operation_not_supported",
			},
			wantZeroPolls: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var polls atomic.Int32
			var issueBody core.PostCertificateIssueJSONRequestBody
			meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
				func(w http.ResponseWriter, req *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					switch req.Method + " " + req.URL.Path {
					case "POST /core/v1/certificates/certificate/issue":
						require.NoError(t,
							json.NewDecoder(req.Body).Decode(&issueBody))
						w.WriteHeader(test.issueStatus)
						body := test.issueBody
						if body == "" {
							body = `{"certificate": {"id": "cert_x"},
								"task": {"id": "task_reissue", "status": "pending"}}`
						}
						_, _ = w.Write([]byte(body))
					case "GET /core/v1/tasks/task":
						require.Equal(t, "task_reissue",
							req.URL.Query().Get("task[id]"))
						n := int(polls.Add(1))
						status := test.taskStatuses[min(n, len(test.taskStatuses))-1]
						_, _ = w.Write([]byte(`{"task": {"id": "task_reissue",
							"status": "` + status + `"}}`))
					default:
						http.Error(w, "unexpected request: "+req.URL.Path,
							http.StatusInternalServerError)
					}
				},
			))
			meta.testMode = true

			a := &CertificateReissueAction{M: meta}
			req := certificateReissueInvokeRequest(t, a, CertificateReissueActionModel{
				CertificateID: types.StringValue("cert_x"),
				Timeout:       test.timeout,
			})
			var progress []string
			resp := &action.InvokeResponse{
				SendProgress: func(event action.InvokeProgressEvent) {
					progress = append(progress, event.Message)
				},
			}
			a.Invoke(context.Background(), req, resp)

			require.Equal(t, "cert_x", *issueBody.Certificate.Id)
			if test.wantZeroPolls {
				require.Zero(t, polls.Load())
			} else {
				require.GreaterOrEqual(t, polls.Load(), test.wantMinPolls)
				require.Len(t, progress, 1)
				require.True(t, strings.Contains(progress[0], "task_reissue"))
			}
			if len(test.wantErr) == 0 {
				require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics)
				return
			}
			require.True(t, resp.Diagnostics.HasError())
			for _, want := range test.wantErr {
				requireDiagnosticContains(t, resp.Diagnostics, want)
			}
		})
	}
}

func certificateReissueInvokeRequest(
	t *testing.T,
	a *CertificateReissueAction,
	model CertificateReissueActionModel,
) action.InvokeRequest {
	t.Helper()
	ctx := context.Background()

	var schemaResp action.SchemaResponse
	a.Schema(ctx, action.SchemaRequest{}, &schemaResp)
	require.False(t, schemaResp.Diagnostics.HasError())

	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	require.False(t, plan.Set(ctx, model).HasError())

	return action.InvokeRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: plan.Raw},
	}
}
