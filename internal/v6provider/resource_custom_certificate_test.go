package v6provider

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	acctest "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jimeh/undent"
	"github.com/stretchr/testify/require"
)

const (
	customCertificateFixtureName    = "tf-acc-test-custom.example.com"
	customCertificateFixtureAltName = "tf-acc-test-custom-alt.example.com"
)

func TestAccKatapultCustomCertificate_minimal(t *testing.T) {
	tt := newTestTools(t)
	certPath, keyPath := customCertificateFixturePaths(t)
	certPEM := readFixture(t, certPath)
	keyPEM := readFixture(t, keyPath)
	res := "katapult_custom_certificate.uploaded"

	acctest.ParallelTest(t, acctest.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultCertificateDestroy(tt),
		Steps: []acctest.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_custom_certificate" "uploaded" {
					  certificate = file("%s")
					  private_key = file("%s")
					}`,
					certPath, keyPath,
				),
				Check: acctest.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultCertificateAttrs(tt, res),
					acctest.TestCheckResourceAttr(
						res, "name", customCertificateFixtureName,
					),
					// Katapult derives name from the CN and additional_names
					// from the remaining SANs.
					acctest.TestCheckResourceAttr(res, "additional_names.#", "1"),
					acctest.TestCheckTypeSetElemAttr(
						res, "additional_names.*", customCertificateFixtureAltName,
					),
					acctest.TestCheckResourceAttr(res, "issuer", "custom"),
					acctest.TestCheckResourceAttr(res, "state", "issued"),
					acctest.TestCheckNoResourceAttr(res, "issue_error"),
					// Configured PEM values are retained verbatim.
					acctest.TestCheckResourceAttr(res, "certificate", certPEM),
					acctest.TestCheckResourceAttr(res, "private_key", keyPEM),
					acctest.TestCheckNoResourceAttr(res, "chain"),
					acctest.TestMatchResourceAttr(
						res, "created_at", rfc3339Pattern,
					),
					acctest.TestMatchResourceAttr(
						res, "expires_at", rfc3339Pattern,
					),
					// Katapult issues custom certificates immediately without
					// a task and leaves last_issued_at null, so it is not
					// asserted here.
				),
			},
			{
				ResourceName:      res,
				ImportState:       true,
				ImportStateVerify: true,
				// Replay serves the redacted private key, so the imported value
				// can never match the configured fixture.
				ImportStateVerifyIgnore: []string{"private_key"},
			},
		},
	})
}

// TestCustomCertificateReadPEMRetention covers the two Read paths for PEM
// inputs: import starts with null values that are filled from the API, while
// configured values are kept even when the API returns something different.
func TestCustomCertificateReadPEMRetention(t *testing.T) {
	const apiResponse = `{
		"certificate": {
			"id": "cert_custom",
			"name": "tf-acc-test-custom.example.com",
			"additional_names": ["tf-acc-test-custom-alt.example.com"],
			"issuer": "custom",
			"state": "issued",
			"created_at": 1700000000,
			"expires_at": 1700086400,
			"last_issued_at": 1700000000,
			"issue_error": null,
			"certificate": "api certificate",
			"chain": "api chain",
			"private_key": "api private key",
			"certificate_api_url": "https://certs.example.test/c/token"
		}
	}`

	tests := map[string]struct {
		state           CustomCertificateResourceModel
		wantCertificate string
		wantPrivateKey  string
		wantChain       string
	}{
		"import populates PEM from the API": {
			state: CustomCertificateResourceModel{
				CertificateModel: CertificateModel{ID: types.StringValue("cert_custom")},
			},
			wantCertificate: "api certificate",
			wantPrivateKey:  "api private key",
			wantChain:       "api chain",
		},
		"configured PEM is retained": {
			state: CustomCertificateResourceModel{
				CertificateModel: CertificateModel{
					ID:          types.StringValue("cert_custom"),
					Certificate: types.StringValue("configured certificate"),
					PrivateKey:  types.StringValue("configured private key"),
					Chain:       types.StringValue("configured chain"),
				},
			},
			wantCertificate: "configured certificate",
			wantPrivateKey:  "configured private key",
			wantChain:       "configured chain",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
				func(w http.ResponseWriter, req *http.Request) {
					require.Equal(t, "/core/v1/certificates/certificate",
						req.URL.Path)
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(apiResponse))
				},
			))
			r := &CustomCertificateResource{M: meta}

			model := test.state
			model.Timeouts = nullResourceTimeouts("delete")
			model.AdditionalNames = types.SetValueMust(
				types.StringType, []attr.Value{},
			)
			state := customCertificateTestState(t, r, model)
			resp := resource.ReadResponse{
				State: tfsdk.State{Schema: state.Schema, Raw: state.Raw},
			}

			r.Read(context.Background(), resource.ReadRequest{State: state}, &resp)

			require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics)
			var got CustomCertificateResourceModel
			require.False(t, resp.State.Get(context.Background(), &got).HasError())
			require.Equal(t, test.wantCertificate, got.Certificate.ValueString())
			require.Equal(t, test.wantPrivateKey, got.PrivateKey.ValueString())
			require.Equal(t, test.wantChain, got.Chain.ValueString())
			require.Equal(t, "tf-acc-test-custom.example.com", got.Name.ValueString())
			require.Equal(t, "issued", got.State.ValueString())
			require.Equal(t, "https://certs.example.test/c/token",
				got.CertificateAPIURL.ValueString())
			require.Equal(t, "2023-11-15T22:13:20Z", got.ExpiresAt.ValueString())
			require.ElementsMatch(t,
				[]attr.Value{types.StringValue("tf-acc-test-custom-alt.example.com")},
				got.AdditionalNames.Elements(),
			)
		})
	}
}

// TestCustomCertificateSchemaRequiresReplaceOnPEMInputs runs the schema's plan
// modifiers for each PEM input and asserts that changing the value forces
// replacement, because the API has no certificate update endpoint.
func TestCustomCertificateSchemaRequiresReplaceOnPEMInputs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	r := &CustomCertificateResource{}
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	require.False(t, schemaResp.Diagnostics.HasError())

	base := CustomCertificateResourceModel{
		CertificateModel: CertificateModel{
			ID:              types.StringValue("cert_custom"),
			Name:            types.StringValue(customCertificateFixtureName),
			AdditionalNames: types.SetValueMust(types.StringType, []attr.Value{}),
			Issuer:          types.StringValue("custom"),
			State:           types.StringValue("issued"),
			Certificate:     types.StringValue("certificate v1"),
			PrivateKey:      types.StringValue("private key v1"),
			Chain:           types.StringValue("chain v1"),
		},
		Timeouts: nullResourceTimeouts("delete"),
	}

	stringAttribute, ok := schemaResp.Schema.Attributes["private_key"].(resourceschema.StringAttribute)
	require.True(t, ok)
	require.True(t, stringAttribute.Sensitive, "private_key must be sensitive")

	for _, attrName := range []string{"certificate", "private_key", "chain"} {
		t.Run(attrName, func(t *testing.T) {
			t.Parallel()

			changed := base
			switch attrName {
			case "certificate":
				changed.Certificate = types.StringValue("certificate v2")
			case "private_key":
				changed.PrivateKey = types.StringValue("private key v2")
			case "chain":
				changed.Chain = types.StringValue("chain v2")
			}

			require.False(t,
				runStringPlanModifiers(t, schemaResp.Schema, attrName, base, base),
				"unchanged %s must not require replacement", attrName,
			)
			require.True(t,
				runStringPlanModifiers(t, schemaResp.Schema, attrName, base, changed),
				"changed %s must require replacement", attrName,
			)
		})
	}
}

func runStringPlanModifiers(
	t *testing.T,
	schema resourceschema.Schema,
	attrName string,
	stateModel CustomCertificateResourceModel,
	planModel CustomCertificateResourceModel,
) bool {
	t.Helper()
	ctx := context.Background()

	state := tfsdk.State{Schema: schema}
	require.False(t, state.Set(ctx, stateModel).HasError())
	plan := tfsdk.Plan{Schema: schema}
	require.False(t, plan.Set(ctx, planModel).HasError())

	var stateValue, planValue types.String
	require.False(t,
		state.GetAttribute(ctx, path.Root(attrName), &stateValue).HasError())
	require.False(t,
		plan.GetAttribute(ctx, path.Root(attrName), &planValue).HasError())

	attribute, ok := schema.Attributes[attrName].(resourceschema.StringAttribute)
	require.True(t, ok, "%s is not a string attribute", attrName)

	req := planmodifier.StringRequest{
		Path:        path.Root(attrName),
		Config:      tfsdk.Config{Schema: schema, Raw: plan.Raw},
		ConfigValue: planValue,
		Plan:        plan,
		PlanValue:   planValue,
		State:       state,
		StateValue:  stateValue,
	}
	resp := &planmodifier.StringResponse{PlanValue: planValue}
	for _, modifier := range attribute.PlanModifiers {
		modifier.PlanModifyString(ctx, req, resp)
	}
	require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics)

	return resp.RequiresReplace
}

func customCertificateTestState(
	t *testing.T,
	r *CustomCertificateResource,
	model CustomCertificateResourceModel,
) tfsdk.State {
	t.Helper()
	ctx := context.Background()

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	require.False(t, schemaResp.Diagnostics.HasError())

	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(ctx, model)
	require.False(t, diags.HasError(), diags.Errors())

	return state
}

func customCertificateFixturePaths(t *testing.T) (string, string) {
	t.Helper()

	dir, err := filepath.Abs(filepath.Join("testdata", "fixtures"))
	require.NoError(t, err)

	return filepath.Join(dir, "custom_certificate.pem"),
		filepath.Join(dir, "custom_certificate.key")
}

func readFixture(t *testing.T, fixturePath string) string {
	t.Helper()

	data, err := os.ReadFile(fixturePath)
	require.NoError(t, err)

	return string(data)
}
