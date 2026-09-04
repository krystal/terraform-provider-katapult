package v6provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
	"github.com/oapi-codegen/nullable"
	"github.com/stretchr/testify/require"
)

func TestCertificateModelFromAPI(t *testing.T) {
	t.Parallel()

	id := "cert_test"
	name := "www.example.com"
	issuer := core.SelfSigned
	createdAt := 1700000000

	t.Run("all fields set", func(t *testing.T) {
		t.Parallel()

		state := core.CertificateStateEnumIssued
		names := []string{"example.com", "alt.example.com"}
		cert := &core.Certificate{
			Id:              &id,
			Name:            &name,
			AdditionalNames: &names,
			Issuer:          &issuer,
			State:           &state,
			IssueError:      nullable.NewNullableWithValue("last failure"),
			Certificate: nullable.NewNullableWithValue(
				"-----BEGIN CERTIFICATE-----\n",
			),
			Chain:      nullable.NewNullableWithValue("chain pem"),
			PrivateKey: nullable.NewNullableWithValue("key pem"),
			CertificateApiUrl: nullable.NewNullableWithValue(
				"https://certs.example.test/c/token",
			),
			CreatedAt:    &createdAt,
			ExpiresAt:    nullable.NewNullableWithValue(createdAt + 86400),
			LastIssuedAt: nullable.NewNullableWithValue(createdAt + 60),
		}

		var m CertificateModel
		m.FromAPI(cert)

		require.Equal(t, "cert_test", m.ID.ValueString())
		require.Equal(t, "www.example.com", m.Name.ValueString())
		require.Equal(t, "self_signed", m.Issuer.ValueString())
		require.Equal(t, "issued", m.State.ValueString())
		require.Equal(t, "last failure", m.IssueError.ValueString())
		require.Equal(t, "-----BEGIN CERTIFICATE-----\n",
			m.Certificate.ValueString())
		require.Equal(t, "chain pem", m.Chain.ValueString())
		require.Equal(t, "key pem", m.PrivateKey.ValueString())
		require.Equal(t, "https://certs.example.test/c/token",
			m.CertificateAPIURL.ValueString())
		require.Equal(t, "2023-11-14T22:13:20Z", m.CreatedAt.ValueString())
		require.Equal(t, "2023-11-15T22:13:20Z", m.ExpiresAt.ValueString())
		require.Equal(t, "2023-11-14T22:14:20Z", m.LastIssuedAt.ValueString())
		require.ElementsMatch(t,
			[]attr.Value{
				types.StringValue("example.com"),
				types.StringValue("alt.example.com"),
			},
			m.AdditionalNames.Elements(),
		)
	})

	t.Run("null and unspecified fields stay null", func(t *testing.T) {
		t.Parallel()

		state := core.CertificateStateEnumPending
		cert := &core.Certificate{
			Id:          &id,
			Name:        &name,
			Issuer:      &issuer,
			State:       &state,
			CreatedAt:   &createdAt,
			IssueError:  nullable.NewNullNullable[string](),
			Certificate: nullable.NewNullNullable[string](),
			ExpiresAt:   nullable.NewNullNullable[int](),
			// Chain, PrivateKey, CertificateApiUrl, and LastIssuedAt are
			// omitted from the response entirely.
		}

		var m CertificateModel
		m.FromAPI(cert)

		require.Equal(t, "pending", m.State.ValueString())
		for attrName, value := range map[string]types.String{
			"issue_error":         m.IssueError,
			"certificate":         m.Certificate,
			"chain":               m.Chain,
			"private_key":         m.PrivateKey,
			"certificate_api_url": m.CertificateAPIURL,
			"expires_at":          m.ExpiresAt,
			"last_issued_at":      m.LastIssuedAt,
		} {
			require.Truef(t, value.IsNull(), "%s should be null", attrName)
			require.Falsef(t, value.IsUnknown(), "%s should be known", attrName)
		}
		require.False(t, m.AdditionalNames.IsNull())
		require.Empty(t, m.AdditionalNames.Elements())
	})
}

func TestUnixSecondsToRFC3339(t *testing.T) {
	t.Parallel()

	require.Equal(t, "1970-01-01T00:00:00Z", unixSecondsToRFC3339(0))
	require.Equal(t, "2023-11-14T22:13:20Z", unixSecondsToRFC3339(1700000000))
}

func TestCheckCertificateIssuer(t *testing.T) {
	t.Parallel()

	id := "cert_test"
	custom := core.Custom
	selfSigned := core.SelfSigned
	future := core.IssuerEnum("future_issuer")

	require.NoError(t, checkCertificateIssuer(
		&core.Certificate{Id: &id, Issuer: &selfSigned}, core.SelfSigned,
	))

	err := checkCertificateIssuer(
		&core.Certificate{Id: &id, Issuer: &custom}, core.SelfSigned,
	)
	require.EqualError(t, err,
		`certificate cert_test has issuer "custom", but `+
			`katapult_self_signed_certificate manages "self_signed" `+
			`certificates; import it with the katapult_custom_certificate `+
			`resource instead`,
	)

	err = checkCertificateIssuer(
		&core.Certificate{Id: &id, Issuer: &future}, core.Custom,
	)
	require.ErrorContains(t, err, `has issuer "future_issuer"`)
	require.NotContains(t, err.Error(), "import it with")

	err = checkCertificateIssuer(&core.Certificate{Id: &id}, core.Custom)
	require.ErrorContains(t, err, "has no issuer")
}

func TestGetCertificateDecodesAPIResponses(t *testing.T) {
	const object = `{"id": "cert_object", "name": "object.example.com",
		"issuer": "custom", "state": "issued", "private_key": null}`

	tests := map[string]struct {
		status   int
		body     string
		wantID   string
		wantErr  string
		notFound bool
	}{
		"object": {
			status: http.StatusOK,
			body:   `{"certificate": ` + object + `}`,
			wantID: "cert_object",
		},
		"array as declared by the generated client": {
			status: http.StatusOK,
			body:   `{"certificate": [` + object + `]}`,
			wantID: "cert_object",
		},
		"empty array": {
			status:   http.StatusOK,
			body:     `{"certificate": []}`,
			notFound: true,
		},
		"not found": {
			status: http.StatusNotFound,
			body: `{"error": {"code": "certificate_not_found",
				"description": "No certificate was found"}}`,
			notFound: true,
		},
		"forbidden": {
			status: http.StatusForbidden,
			body: `{"error": {"code": "permission_denied",
				"description": "Not permitted"}}`,
			wantErr: "permission_denied: Not permitted",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var gotQuery string
			meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
				func(w http.ResponseWriter, req *http.Request) {
					gotQuery = req.URL.Query().Get("certificate[id]")
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(test.status)
					_, _ = w.Write([]byte(test.body))
				},
			))

			cert, err := getCertificate(context.Background(), meta, "cert_lookup")

			require.Equal(t, "cert_lookup", gotQuery)
			switch {
			case test.notFound:
				require.ErrorIs(t, err, core.ErrNotFound)
			case test.wantErr != "":
				require.EqualError(t, err, test.wantErr)
			default:
				require.NoError(t, err)
				require.Equal(t, test.wantID, *cert.Id)
				require.True(t, cert.PrivateKey.IsNull())
			}
		})
	}
}

func TestDeleteCertificate(t *testing.T) {
	tests := map[string]struct {
		status  int
		body    string
		wantErr []string
	}{
		"deleted": {
			status: http.StatusOK,
			body:   `{"certificate": {"id": "cert_delete"}}`,
		},
		"already gone": {
			status: http.StatusNotFound,
			body: `{"error": {"code": "certificate_not_found",
				"description": "No certificate was found"}}`,
		},
		"referenced by a load balancer rule": {
			status: http.StatusConflict,
			body: `{"error": {"code": "deletion_restricted",
				"description": "This certificate cannot be deleted",
				"detail": {"errors": ["It is in use by a load balancer rule"]}}}`,
			wantErr: []string{
				"cert_delete cannot be deleted while a load balancer rule",
				"create_before_destroy",
				"deletion_restricted: This certificate cannot be deleted",
			},
		},
		"other failure": {
			status: http.StatusForbidden,
			body: `{"error": {"code": "permission_denied",
				"description": "Not permitted"}}`,
			wantErr: []string{"permission_denied: Not permitted"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var gotBody core.DeleteCertificateJSONRequestBody
			meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
				func(w http.ResponseWriter, req *http.Request) {
					require.Equal(t, http.MethodDelete, req.Method)
					require.Equal(t, "/core/v1/certificates/certificate",
						req.URL.Path)
					require.NoError(t,
						json.NewDecoder(req.Body).Decode(&gotBody))
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(test.status)
					_, _ = w.Write([]byte(test.body))
				},
			))

			err := deleteCertificate(context.Background(), meta, "cert_delete")

			require.Equal(t, "cert_delete", *gotBody.Certificate.Id)
			if len(test.wantErr) == 0 {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			for _, want := range test.wantErr {
				require.ErrorContains(t, err, want)
			}
			if test.status == http.StatusConflict {
				var apiErr *GenericAPIError
				require.True(t, errors.As(err, &apiErr))
			}
		})
	}
}
