package v6provider

import (
	"context"
	"encoding/json"
	"net/http"
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

// TestAccKatapultCertificateResetToken_self_signed runs the reset token action
// from a terraform_data action_trigger through the muxed provider. The
// rotated certificate_api_url is not asserted because cassettes redact it.
func TestAccKatapultCertificateResetToken_self_signed(t *testing.T) {
	tt := newTestTools(t)
	name := testAccCertificateHostname(tt)
	res := "katapult_self_signed_certificate.web"

	config := undent.Stringf(`
		resource "katapult_self_signed_certificate" "web" {
		  name = "%s"
		}

		action "katapult_certificate_reset_token" "web" {
		  config {
		    certificate_id = katapult_self_signed_certificate.web.id
		  }
		}

		resource "terraform_data" "reset_token" {
		  lifecycle {
		    action_trigger {
		      events  = [after_create]
		      actions = [action.katapult_certificate_reset_token.web]
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
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultCertificateAttrs(tt, res),
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

func TestCertificateResetTokenActionInvoke(t *testing.T) {
	tests := map[string]struct {
		status  int
		body    string
		wantErr []string
	}{
		"success": {
			status: http.StatusOK,
			body:   `{"certificate": {"id": "cert_x"}}`,
		},
		"not found": {
			status: http.StatusNotFound,
			body: `{"error": {"code": "certificate_not_found",
				"description": "No certificate was found"}}`,
			wantErr: []string{
				"Certificate Reset Token Error",
				"certificate_not_found: No certificate was found",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var requests atomic.Int32
			var body core.PostCertificateResetTokenJSONRequestBody
			meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
				func(w http.ResponseWriter, req *http.Request) {
					requests.Add(1)
					require.Equal(t, http.MethodPost, req.Method)
					require.Equal(t,
						"/core/v1/certificates/certificate/reset_token",
						req.URL.Path)
					require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(test.status)
					_, _ = w.Write([]byte(test.body))
				},
			))

			a := &CertificateResetTokenAction{M: meta}
			ctx := context.Background()
			var schemaResp action.SchemaResponse
			a.Schema(ctx, action.SchemaRequest{}, &schemaResp)
			plan := tfsdk.Plan{Schema: schemaResp.Schema}
			require.False(t, plan.Set(ctx, CertificateResetTokenActionModel{
				CertificateID: types.StringValue("cert_x"),
			}).HasError())

			resp := &action.InvokeResponse{}
			a.Invoke(ctx, action.InvokeRequest{
				Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: plan.Raw},
			}, resp)

			require.Equal(t, int32(1), requests.Load())
			require.Equal(t, "cert_x", *body.Certificate.Id)
			if len(test.wantErr) == 0 {
				require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics)
				return
			}
			for _, want := range test.wantErr {
				requireDiagnosticContains(t, resp.Diagnostics, want)
			}
		})
	}
}
