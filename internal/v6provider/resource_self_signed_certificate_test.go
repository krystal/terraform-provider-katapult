package v6provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/defaults"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/next/core"
	"github.com/stretchr/testify/require"
)

var rfc3339Pattern = regexp.MustCompile(
	`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`,
)

func init() { //nolint:gochecknoinits
	resource.AddTestSweepers("katapult_certificate", &resource.Sweeper{
		Name:         "katapult_certificate",
		F:            testSweepCertificates,
		Dependencies: []string{"katapult_load_balancer"},
	})
}

func testSweepCertificates(_ string) error {
	pageSize := 100

	m := sweepMeta()
	ctx := context.TODO()

	var certs []core.GetOrganizationCertificates200ResponseCertificates
	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		res, err := m.Core.GetOrganizationCertificatesWithResponse(ctx,
			&core.GetOrganizationCertificatesParams{
				OrganizationSubDomain: &m.confOrganization,
				Page:                  &pageNum,
				PerPage:               &pageSize,
			})
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				return nil
			}
			return err
		}

		totalPages, _ = res.JSON200.Pagination.TotalPages.Get()
		certs = append(certs, res.JSON200.Certificates...)
	}

	for _, cert := range certs {
		if cert.Name == nil || cert.Id == nil ||
			!strings.HasPrefix(*cert.Name, testAccResourceNamePrefix) {
			continue
		}

		m.Logger.Info("deleting certificate", "id", *cert.Id, "name", *cert.Name)

		if err := deleteCertificate(ctx, m, *cert.Id); err != nil {
			return err
		}
	}

	return nil
}

func TestAccKatapultSelfSignedCertificate_minimal(t *testing.T) {
	tt := newTestTools(t)
	name := testAccCertificateHostname(tt)
	res := "katapult_self_signed_certificate.web"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultCertificateDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_self_signed_certificate" "web" {
					  name = "%s"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultCertificateAttrs(tt, res),
					resource.TestCheckResourceAttr(res, "name", name),
					resource.TestCheckResourceAttr(res, "issuer", "self_signed"),
					resource.TestCheckResourceAttr(res, "state", "issued"),
					resource.TestCheckResourceAttr(res, "additional_names.#", "0"),
					resource.TestCheckNoResourceAttr(res, "issue_error"),
					resource.TestMatchResourceAttr(res, "certificate",
						regexp.MustCompile(`^-----BEGIN CERTIFICATE-----`)),
					resource.TestCheckNoResourceAttr(res, "chain"),
					// The test organization's token can view private material,
					// so both sensitive outputs are populated. Cassettes hold
					// redacted placeholders.
					resource.TestCheckResourceAttrSet(res, "private_key"),
					resource.TestCheckResourceAttrSet(res, "certificate_api_url"),
					resource.TestMatchResourceAttr(res, "created_at", rfc3339Pattern),
					resource.TestMatchResourceAttr(res, "expires_at", rfc3339Pattern),
					resource.TestMatchResourceAttr(res, "last_issued_at", rfc3339Pattern),
				),
			},
			{
				ResourceName:      res,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultSelfSignedCertificate_additional_names(t *testing.T) {
	tt := newTestTools(t)
	base := strings.ToLower(tt.ResourceName())
	name := base + ".example.com"
	renamed := base + "-renamed.example.com"
	altNames := []string{
		"alt1." + base + ".example.com",
		"alt2." + base + ".example.com",
	}
	res := "katapult_self_signed_certificate.web"

	config := func(name string) string {
		return undent.Stringf(`
			resource "katapult_self_signed_certificate" "web" {
			  name             = "%s"
			  additional_names = ["%s", "%s"]
			}`,
			name, altNames[0], altNames[1],
		)
	}

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultCertificateDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: config(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultCertificateAttrs(tt, res),
					resource.TestCheckResourceAttr(res, "name", name),
					resource.TestCheckResourceAttr(res, "state", "issued"),
					resource.TestCheckResourceAttr(res, "additional_names.#", "2"),
					resource.TestCheckTypeSetElemAttr(
						res, "additional_names.*", altNames[0],
					),
					resource.TestCheckTypeSetElemAttr(
						res, "additional_names.*", altNames[1],
					),
				),
			},
			{
				Config: config(renamed),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(
							res, plancheck.ResourceActionReplace,
						),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultCertificateAttrs(tt, res),
					resource.TestCheckResourceAttr(res, "name", renamed),
					resource.TestCheckResourceAttr(res, "state", "issued"),
					resource.TestCheckResourceAttr(res, "additional_names.#", "2"),
				),
			},
			{
				ResourceName:      res,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestSelfSignedCertificateCreateRetainsStateWhenIssuanceFails proves that a
// certificate whose issuance task fails is kept in state with its ID, so
// Terraform taints it instead of orphaning it and creating another.
func TestSelfSignedCertificateCreateRetainsStateWhenIssuanceFails(
	t *testing.T,
) {
	const name = "tf-acc-test-failed.example.com"

	var createBody core.PostOrganizationCertificatesJSONRequestBody
	var certificateReads atomic.Int32
	meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch req.Method + " " + req.URL.Path {
			case "POST /core/v1/organizations/organization/certificates":
				require.NoError(t,
					json.NewDecoder(req.Body).Decode(&createBody))
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{
					"certificate": {
						"id": "cert_failed",
						"name": "` + name + `",
						"additional_names": [],
						"issuer": "self_signed",
						"state": "pending",
						"created_at": 1700000000,
						"expires_at": null,
						"last_issued_at": null,
						"issue_error": null,
						"certificate": null,
						"chain": null,
						"private_key": null,
						"certificate_api_url": null
					},
					"task": {"id": "task_failed", "status": "pending"}
				}`))
			case "GET /core/v1/tasks/task":
				_, _ = w.Write([]byte(`{
					"task": {"id": "task_failed", "status": "failed"}
				}`))
			case "GET /core/v1/certificates/certificate":
				certificateReads.Add(1)
				_, _ = w.Write([]byte(`{
					"certificate": {
						"id": "cert_failed",
						"name": "` + name + `",
						"additional_names": [],
						"issuer": "self_signed",
						"state": "issue_failed",
						"created_at": 1700000000,
						"expires_at": null,
						"last_issued_at": null,
						"issue_error": "injected issuance failure",
						"certificate": null,
						"chain": null,
						"private_key": null,
						"certificate_api_url": null
					}
				}`))
			default:
				http.Error(w, "unexpected request: "+req.Method+" "+req.URL.Path,
					http.StatusInternalServerError)
			}
		},
	))
	meta.testMode = true

	r := &SelfSignedCertificateResource{M: meta}
	plan := SelfSignedCertificateResourceModel{
		CertificateModel: unknownCertificateModel(),
		Timeouts:         nullResourceTimeouts("create", "delete"),
	}
	plan.Name = types.StringValue(name)
	req, resp := objectStorageCreateOperation(t, r.Schema, plan)

	r.Create(context.Background(), req, &resp)

	require.Equal(t, core.SelfSigned, createBody.Properties.Issuer)
	require.Equal(t, name, *createBody.Properties.Name)
	require.Equal(t, int32(1), certificateReads.Load())
	requireDiagnosticContains(t, resp.Diagnostics, "Certificate Issuance Error")
	requireDiagnosticContains(t, resp.Diagnostics, "task failed")
	requireDiagnosticContains(t, resp.Diagnostics,
		"issue_error: injected issuance failure")

	var state SelfSignedCertificateResourceModel
	require.False(t, resp.State.Get(context.Background(), &state).HasError())
	require.Equal(t, "cert_failed", state.ID.ValueString())
	require.Equal(t, name, state.Name.ValueString())
	require.Equal(t, "issue_failed", state.State.ValueString())
	require.Equal(t, "injected issuance failure", state.IssueError.ValueString())
	require.Equal(t, "2023-11-14T22:13:20Z", state.CreatedAt.ValueString())
	requireKnownNullString(t, state.Certificate)
	requireKnownNullString(t, state.PrivateKey)
	requireKnownNullString(t, state.ExpiresAt)
	require.False(t, state.AdditionalNames.IsUnknown())
	require.Empty(t, state.AdditionalNames.Elements())
}

// TestSelfSignedCertificateAdditionalNamesDefault proves that an omitted
// additional_names plans as an empty set, so removing previously configured
// names is a real change that forces replacement instead of silently keeping
// the old names.
func TestSelfSignedCertificateAdditionalNamesDefault(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	var schemaResp frameworkresource.SchemaResponse
	(&SelfSignedCertificateResource{}).Schema(
		ctx, frameworkresource.SchemaRequest{}, &schemaResp,
	)
	require.False(t, schemaResp.Diagnostics.HasError())

	attribute, ok := schemaResp.Schema.Attributes["additional_names"].(resourceschema.SetAttribute)
	require.True(t, ok)
	require.NotNil(t, attribute.Default, "additional_names must have a default")

	var defaultResp defaults.SetResponse
	attribute.Default.DefaultSet(ctx, defaults.SetRequest{}, &defaultResp)
	require.False(t, defaultResp.Diagnostics.HasError())
	require.False(t, defaultResp.PlanValue.IsNull())
	require.False(t, defaultResp.PlanValue.IsUnknown())
	require.Empty(t, defaultResp.PlanValue.Elements())

	withNames := SelfSignedCertificateResourceModel{
		CertificateModel: unknownCertificateModel(),
		Timeouts:         nullResourceTimeouts("create", "delete"),
	}
	withNames.ID = types.StringValue("cert_names")
	withNames.Name = types.StringValue("tf-acc-test-default.example.com")
	withNames.AdditionalNames = types.SetValueMust(types.StringType, []attr.Value{
		types.StringValue("alt.tf-acc-test-default.example.com"),
	})
	withoutNames := withNames
	withoutNames.AdditionalNames = defaultResp.PlanValue

	require.False(t,
		runSetPlanModifiers(t, schemaResp.Schema, "additional_names",
			withNames, withNames),
		"unchanged additional_names must not require replacement",
	)
	require.True(t,
		runSetPlanModifiers(t, schemaResp.Schema, "additional_names",
			withNames, withoutNames),
		"removing additional_names must require replacement",
	)
}

func runSetPlanModifiers(
	t *testing.T,
	schema resourceschema.Schema,
	attrName string,
	stateModel SelfSignedCertificateResourceModel,
	planModel SelfSignedCertificateResourceModel,
) bool {
	t.Helper()
	ctx := context.Background()

	state := tfsdk.State{Schema: schema}
	require.False(t, state.Set(ctx, stateModel).HasError())
	plan := tfsdk.Plan{Schema: schema}
	require.False(t, plan.Set(ctx, planModel).HasError())

	var stateValue, planValue types.Set
	require.False(t,
		state.GetAttribute(ctx, path.Root(attrName), &stateValue).HasError())
	require.False(t,
		plan.GetAttribute(ctx, path.Root(attrName), &planValue).HasError())

	attribute, ok := schema.Attributes[attrName].(resourceschema.SetAttribute)
	require.True(t, ok, "%s is not a set attribute", attrName)

	req := planmodifier.SetRequest{
		Path:        path.Root(attrName),
		Config:      tfsdk.Config{Schema: schema, Raw: plan.Raw},
		ConfigValue: planValue,
		Plan:        plan,
		PlanValue:   planValue,
		State:       state,
		StateValue:  stateValue,
	}
	resp := &planmodifier.SetResponse{PlanValue: planValue}
	for _, modifier := range attribute.PlanModifiers {
		modifier.PlanModifySet(ctx, req, resp)
	}
	require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics)

	return resp.RequiresReplace
}

//
// Helpers
//

// testAccCertificateHostname returns a lowercase hostname with a dotted
// suffix, as Katapult's hostname validation requires.
func testAccCertificateHostname(tt *testTools) string {
	return strings.ToLower(tt.ResourceName()) + ".example.com"
}

// unknownCertificateModel returns the shared model as Terraform plans it on
// create: every computed attribute unknown.
func unknownCertificateModel() CertificateModel {
	return CertificateModel{
		ID:                types.StringUnknown(),
		Name:              types.StringUnknown(),
		AdditionalNames:   types.SetUnknown(types.StringType),
		Issuer:            types.StringUnknown(),
		State:             types.StringUnknown(),
		IssueError:        types.StringUnknown(),
		Certificate:       types.StringUnknown(),
		Chain:             types.StringUnknown(),
		PrivateKey:        types.StringUnknown(),
		CertificateAPIURL: types.StringUnknown(),
		CreatedAt:         types.StringUnknown(),
		ExpiresAt:         types.StringUnknown(),
		LastIssuedAt:      types.StringUnknown(),
	}
}

func nullResourceTimeouts(names ...string) timeouts.Value {
	attrTypes := make(map[string]attr.Type, len(names))
	for _, name := range names {
		attrTypes[name] = types.StringType
	}

	return timeouts.Value{Object: types.ObjectNull(attrTypes)}
}

func testAccCheckKatapultCertificateAttrs(
	tt *testTools,
	res string,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("resource not found: %s", res)
		}

		cert, err := getCertificate(tt.Ctx, tt.Meta, rs.Primary.ID)
		if err != nil {
			return err
		}

		var additionalNames []string
		if cert.AdditionalNames != nil {
			additionalNames = *cert.AdditionalNames
		}

		tfs := []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(res, "id", *cert.Id),
			resource.TestCheckResourceAttr(res, "name", *cert.Name),
			resource.TestCheckResourceAttr(res, "issuer", string(*cert.Issuer)),
			resource.TestCheckResourceAttr(res, "state", string(*cert.State)),
			resource.TestCheckResourceAttr(
				res, "additional_names.#", strconv.Itoa(len(additionalNames)),
			),
			resource.TestCheckResourceAttr(
				res, "created_at", unixSecondsToRFC3339(*cert.CreatedAt),
			),
		}
		for _, name := range additionalNames {
			tfs = append(tfs, resource.TestCheckTypeSetElemAttr(
				res, "additional_names.*", name,
			))
		}
		if expiresAt, err := cert.ExpiresAt.Get(); err == nil {
			tfs = append(tfs, resource.TestCheckResourceAttr(
				res, "expires_at", unixSecondsToRFC3339(expiresAt),
			))
		}

		return resource.ComposeAggregateTestCheckFunc(tfs...)(s)
	}
}

func testAccCheckKatapultCertificateDestroy(
	tt *testTools,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			switch rs.Type {
			case "katapult_self_signed_certificate",
				"katapult_custom_certificate":
			default:
				continue
			}

			cert, err := getCertificate(tt.Ctx, tt.Meta, rs.Primary.ID)
			if err == nil {
				return fmt.Errorf(
					"%s %s (%s) was not destroyed",
					rs.Type, rs.Primary.ID, *cert.Name,
				)
			}
			if !errors.Is(err, core.ErrNotFound) {
				return err
			}
		}

		return nil
	}
}
