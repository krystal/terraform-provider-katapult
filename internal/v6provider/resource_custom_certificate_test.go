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

// customCertificateAPIResponse is a GET certificate response whose PEM values
// differ from any configured value, so retention and import can be told apart.
const customCertificateAPIResponse = `{
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

// TestCustomCertificateReadPEMRetention asserts that Read never refreshes the
// PEM inputs from the API: configured values are kept even when the API
// returns something different, and a null chain stays null even when the API
// reports one, which would otherwise force a replacement on every plan.
func TestCustomCertificateReadPEMRetention(t *testing.T) {
	tests := map[string]struct {
		state           CustomCertificateResourceModel
		wantCertificate types.String
		wantPrivateKey  types.String
		wantChain       types.String
	}{
		"configured PEM is retained": {
			state: CustomCertificateResourceModel{
				CertificateModel: CertificateModel{
					ID:          types.StringValue("cert_custom"),
					Certificate: types.StringValue("configured certificate"),
					PrivateKey:  types.StringValue("configured private key"),
					Chain:       types.StringValue("configured chain"),
				},
			},
			wantCertificate: types.StringValue("configured certificate"),
			wantPrivateKey:  types.StringValue("configured private key"),
			wantChain:       types.StringValue("configured chain"),
		},
		"null chain stays null": {
			state: CustomCertificateResourceModel{
				CertificateModel: CertificateModel{
					ID:          types.StringValue("cert_custom"),
					Certificate: types.StringValue("configured certificate"),
					PrivateKey:  types.StringValue("configured private key"),
				},
			},
			wantCertificate: types.StringValue("configured certificate"),
			wantPrivateKey:  types.StringValue("configured private key"),
			wantChain:       types.StringNull(),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			r := &CustomCertificateResource{M: customCertificateTestMeta(t)}

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
			require.Equal(t, test.wantCertificate, got.Certificate)
			require.Equal(t, test.wantPrivateKey, got.PrivateKey)
			require.Equal(t, test.wantChain, got.Chain)
			requireCustomCertificateComputedAttrs(t, got)
		})
	}
}

// TestCustomCertificateImportStateFillsPEM asserts that import is the one
// path that populates certificate, private_key, and chain from the API, along
// with the ID and the shared computed outputs.
func TestCustomCertificateImportStateFillsPEM(t *testing.T) {
	ctx := context.Background()
	r := &CustomCertificateResource{M: customCertificateTestMeta(t)}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	require.False(t, schemaResp.Diagnostics.HasError())

	resp := resource.ImportStateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema},
	}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "cert_custom"}, &resp)

	require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics)
	var got CustomCertificateResourceModel
	require.False(t, resp.State.Get(ctx, &got).HasError())
	require.Equal(t, "cert_custom", got.ID.ValueString())
	require.Equal(t, types.StringValue("api certificate"), got.Certificate)
	require.Equal(t, types.StringValue("api private key"), got.PrivateKey)
	require.Equal(t, types.StringValue("api chain"), got.Chain)
	require.True(t, got.Timeouts.IsNull(), "imported timeouts must be null")
	requireCustomCertificateComputedAttrs(t, got)
}

func customCertificateTestMeta(t *testing.T) *Meta {
	t.Helper()

	return newObjectStorageFailureTestMeta(t, http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			require.Equal(t, "/core/v1/certificates/certificate", req.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(customCertificateAPIResponse))
		},
	))
}

func requireCustomCertificateComputedAttrs(
	t *testing.T,
	got CustomCertificateResourceModel,
) {
	t.Helper()

	require.Equal(t, "tf-acc-test-custom.example.com", got.Name.ValueString())
	require.Equal(t, "custom", got.Issuer.ValueString())
	require.Equal(t, "issued", got.State.ValueString())
	require.Equal(t, "https://certs.example.test/c/token",
		got.CertificateAPIURL.ValueString())
	require.Equal(t, "2023-11-15T22:13:20Z", got.ExpiresAt.ValueString())
	require.ElementsMatch(t,
		[]attr.Value{types.StringValue("tf-acc-test-custom-alt.example.com")},
		got.AdditionalNames.Elements(),
	)
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
