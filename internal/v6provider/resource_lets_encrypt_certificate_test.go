package v6provider

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
	"github.com/stretchr/testify/require"
)

const (
	letsEncryptTestName = "tf-acc-test-le.example.com"
	pendingState        = string(core.CertificateStateEnumPending)
	issuedState         = string(core.CertificateStateEnumIssued)
)

func letsEncryptCertificateJSON(state string, issued bool) string {
	issuedFields := `"expires_at": null, "last_issued_at": null,
		"certificate": null, "chain": null, "private_key": null,
		"certificate_api_url": null`
	if issued {
		issuedFields = `"expires_at": 1707776000, "last_issued_at": 1700000000,
			"certificate": "-----BEGIN CERTIFICATE-----\nle\n",
			"chain": "-----BEGIN CERTIFICATE-----\nchain\n",
			"private_key": "key", "certificate_api_url": "https://certs/x"`
	}

	return `{
		"id": "cert_le",
		"name": "` + letsEncryptTestName + `",
		"additional_names": [],
		"issuer": "lets_encrypt",
		"authorization_method": "http",
		"state": "` + state + `",
		"created_at": 1700000000,
		"issue_error": null,
		` + issuedFields + `
	}`
}

func letsEncryptPlan(
	method string,
	wait bool,
	additionalNames ...string,
) LetsEncryptCertificateResourceModel {
	names := make([]attr.Value, 0, len(additionalNames))
	for _, name := range additionalNames {
		names = append(names, types.StringValue(name))
	}

	plan := LetsEncryptCertificateResourceModel{
		CertificateModel:    unknownCertificateModel(),
		AuthorizationMethod: types.StringValue(method),
		WaitForIssuance:     types.BoolValue(wait),
		Timeouts:            nullResourceTimeouts("create", "delete"),
	}
	plan.Name = types.StringValue(letsEncryptTestName)
	plan.AdditionalNames = types.SetValueMust(types.StringType, names)

	return plan
}

func TestLetsEncryptCertificateCreateRequestBody(t *testing.T) {
	tests := map[string]struct {
		method    string
		names     []string
		wantNames *[]string
	}{
		"dns without additional names":  {method: "dns"},
		"http without additional names": {method: "http"},
		"dns with additional names": {
			method:    "dns",
			names:     []string{"a.example.com", "b.example.com"},
			wantNames: &[]string{"a.example.com", "b.example.com"},
		},
		"http with additional names": {
			method:    "http",
			names:     []string{"a.example.com"},
			wantNames: &[]string{"a.example.com"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var body core.PostOrganizationCertificatesJSONRequestBody
			var rawBody map[string]map[string]json.RawMessage
			meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
				func(w http.ResponseWriter, req *http.Request) {
					require.Equal(t, http.MethodPost, req.Method)
					require.Equal(t,
						"/core/v1/organizations/organization/certificates",
						req.URL.Path)
					raw, err := readAllBody(req)
					require.NoError(t, err)
					require.NoError(t, json.Unmarshal(raw, &body))
					require.NoError(t, json.Unmarshal(raw, &rawBody))
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_, _ = w.Write([]byte(`{"certificate": ` +
						letsEncryptCertificateJSON(pendingState, false) +
						`, "task": {"id": "task_le", "status": "pending"}}`))
				},
			))
			meta.testMode = true

			r := &LetsEncryptCertificateResource{M: meta}
			req, resp := objectStorageCreateOperation(
				t, r.Schema, letsEncryptPlan(test.method, false, test.names...),
			)
			r.Create(context.Background(), req, &resp)
			require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics)

			require.Equal(t, "test-organization", *body.Organization.SubDomain)
			require.Equal(t, core.LetsEncrypt, body.Properties.Issuer)
			require.Equal(t, letsEncryptTestName, *body.Properties.Name)
			require.Equal(t, test.method, *body.Properties.AuthorizationMethod)
			require.Nil(t, body.Properties.Certificate)
			require.Nil(t, body.Properties.PrivateKey)
			require.Nil(t, body.Properties.Chain)
			if test.wantNames == nil {
				_, present := rawBody["properties"]["additional_names"]
				require.False(t, present,
					"additional_names must be omitted when empty")
			} else {
				require.NotNil(t, body.Properties.AdditionalNames)
				require.ElementsMatch(t, *test.wantNames,
					*body.Properties.AdditionalNames)
			}
		})
	}
}

func TestLetsEncryptCertificateAuthorizationMethodValidation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	var schemaResp frameworkresource.SchemaResponse
	(&LetsEncryptCertificateResource{}).Schema(
		ctx, frameworkresource.SchemaRequest{}, &schemaResp,
	)
	attribute, ok := schemaResp.Schema.Attributes["authorization_method"].(resourceschema.StringAttribute)
	require.True(t, ok)
	require.NotEmpty(t, attribute.Validators)

	for value, wantErr := range map[string]bool{
		"dns": false, "http": false, "email": true, "DNS": true, "": true,
	} {
		resp := &validator.StringResponse{}
		for _, v := range attribute.Validators {
			v.ValidateString(ctx, validator.StringRequest{
				ConfigValue: types.StringValue(value),
			}, resp)
		}
		require.Equalf(t, wantErr, resp.Diagnostics.HasError(),
			"authorization_method %q", value)
	}
}

func TestLetsEncryptCertificateValidateConfigRejectsWildcardWithHTTP(
	t *testing.T,
) {
	t.Parallel()

	tests := map[string]struct {
		method    types.String
		name      string
		names     []string
		wantError string
	}{
		"http wildcard name": {
			method: types.StringValue("http"), name: "*.example.com",
			wantError: "name",
		},
		"http wildcard additional name": {
			method: types.StringValue("http"), name: "www.example.com",
			names:     []string{"api.example.com", "*.example.com"},
			wantError: "additional_names",
		},
		"http plain names": {
			method: types.StringValue("http"), name: "www.example.com",
			names: []string{"api.example.com"},
		},
		"dns wildcard": {
			method: types.StringValue("dns"), name: "*.example.com",
			names: []string{"*.api.example.com"},
		},
		"unknown method wildcard": {
			method: types.StringUnknown(), name: "*.example.com",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			r := LetsEncryptCertificateResource{}
			var schemaResp frameworkresource.SchemaResponse
			r.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResp)

			model := letsEncryptPlan("dns", true, test.names...)
			model.Name = types.StringValue(test.name)
			model.AuthorizationMethod = test.method
			plan := tfsdk.Plan{Schema: schemaResp.Schema}
			require.False(t, plan.Set(ctx, model).HasError())

			resp := &frameworkresource.ValidateConfigResponse{}
			r.ValidateConfig(ctx, frameworkresource.ValidateConfigRequest{
				Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: plan.Raw},
			}, resp)

			if test.wantError == "" {
				require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics)
				return
			}
			require.True(t, resp.Diagnostics.HasError())
			requireDiagnosticContains(t, resp.Diagnostics,
				"Wildcard Requires DNS Authorization")
			withPath, ok := resp.Diagnostics.Errors()[0].(diag.DiagnosticWithPath)
			require.True(t, ok, "diagnostic must carry an attribute path")
			require.Equal(t, test.wantError, withPath.Path().String())
		})
	}
}

func TestLetsEncryptCertificateCreateWithoutWait(t *testing.T) {
	var taskRequests, certificateRequests atomic.Int32
	meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch req.URL.Path {
			case "/core/v1/organizations/organization/certificates":
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"certificate": ` +
					letsEncryptCertificateJSON(pendingState, false) +
					`, "task": {"id": "task_le", "status": "pending"}}`))
			case "/core/v1/tasks/task":
				taskRequests.Add(1)
			case "/core/v1/certificates/certificate":
				certificateRequests.Add(1)
			default:
				http.Error(w, "unexpected request: "+req.URL.Path,
					http.StatusInternalServerError)
			}
		},
	))
	meta.testMode = true

	r := &LetsEncryptCertificateResource{M: meta}
	req, resp := objectStorageCreateOperation(
		t, r.Schema, letsEncryptPlan("http", false),
	)
	r.Create(context.Background(), req, &resp)

	require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics)
	require.Zero(t, taskRequests.Load(), "no task request when not waiting")
	require.Zero(t, certificateRequests.Load())

	var state LetsEncryptCertificateResourceModel
	require.False(t, resp.State.Get(context.Background(), &state).HasError())
	require.Equal(t, "cert_le", state.ID.ValueString())
	require.Equal(t, pendingState, state.State.ValueString())
	require.Equal(t, "http", state.AuthorizationMethod.ValueString())
	require.False(t, state.WaitForIssuance.ValueBool())
	requireKnownNullString(t, state.Certificate)
	requireKnownNullString(t, state.ExpiresAt)
	require.False(t, state.AdditionalNames.IsUnknown())
}

func TestLetsEncryptCertificateCreateWaitsForIssuance(t *testing.T) {
	var taskRequests atomic.Int32
	meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch req.URL.Path {
			case "/core/v1/organizations/organization/certificates":
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"certificate": ` +
					letsEncryptCertificateJSON(pendingState, false) +
					`, "task": {"id": "task_le", "status": "pending"}}`))
			case "/core/v1/tasks/task":
				require.Equal(t, "task_le", req.URL.Query().Get("task[id]"))
				status := "running"
				if taskRequests.Add(1) >= 2 {
					status = "completed"
				}
				_, _ = w.Write([]byte(`{"task": {"id": "task_le", "status": "` +
					status + `"}}`))
			case "/core/v1/certificates/certificate":
				_, _ = w.Write([]byte(`{"certificate": ` +
					letsEncryptCertificateJSON(issuedState, true) + `}`))
			default:
				http.Error(w, "unexpected request: "+req.URL.Path,
					http.StatusInternalServerError)
			}
		},
	))
	meta.testMode = true

	r := &LetsEncryptCertificateResource{M: meta}
	req, resp := objectStorageCreateOperation(
		t, r.Schema, letsEncryptPlan("dns", true),
	)
	r.Create(context.Background(), req, &resp)

	require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics)
	require.GreaterOrEqual(t, taskRequests.Load(), int32(2))

	var state LetsEncryptCertificateResourceModel
	require.False(t, resp.State.Get(context.Background(), &state).HasError())
	require.Equal(t, "cert_le", state.ID.ValueString())
	require.Equal(t, "issued", state.State.ValueString())
	require.True(t, state.WaitForIssuance.ValueBool())
	require.Equal(t, "-----BEGIN CERTIFICATE-----\nle\n",
		state.Certificate.ValueString())
	require.Equal(t, "-----BEGIN CERTIFICATE-----\nchain\n",
		state.Chain.ValueString())
	require.Equal(t, "key", state.PrivateKey.ValueString())
	require.Equal(t, "https://certs/x", state.CertificateAPIURL.ValueString())
	require.Equal(t, "2024-02-12T22:13:20Z", state.ExpiresAt.ValueString())
	require.Equal(t, "2023-11-14T22:13:20Z", state.LastIssuedAt.ValueString())
	requireKnownNullString(t, state.IssueError)
}
