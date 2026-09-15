package v6provider

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
	"github.com/stretchr/testify/require"
)

func TestAccKatapultDataSourceCertificates_minimal(t *testing.T) {
	tt := newTestTools(t)
	name := testAccCertificateHostname(tt)
	res := "katapult_self_signed_certificate.web"
	data := "data.katapult_certificates.all"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultCertificateDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_self_signed_certificate" "web" {
					  name = "%s"
					}

					data "katapult_certificates" "all" {
					  depends_on = [katapult_self_signed_certificate.web]
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultCertificateAttrs(tt, res),
					testAccCheckKatapultCertificatesContains(data, res),
				),
			},
		},
	})
}

// testAccCheckKatapultCertificatesContains asserts that the plural data source
// lists the given certificate resource with matching summary fields.
func testAccCheckKatapultCertificatesContains(
	data string,
	res string,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		cert, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("resource not found: %s", res)
		}
		list, ok := s.RootModule().Resources[data]
		if !ok {
			return fmt.Errorf("data source not found: %s", data)
		}

		count, err := strconv.Atoi(list.Primary.Attributes["certificates.#"])
		if err != nil {
			return fmt.Errorf("certificates.# is not a number: %w", err)
		}
		if count < 1 {
			return fmt.Errorf("%s lists no certificates", data)
		}

		for i := 0; i < count; i++ {
			prefix := fmt.Sprintf("certificates.%d.", i)
			if list.Primary.Attributes[prefix+"id"] != cert.Primary.ID {
				continue
			}

			for _, key := range []string{
				"name", "issuer", "state", "expires_at", "last_issued_at",
			} {
				want := cert.Primary.Attributes[key]
				got := list.Primary.Attributes[prefix+key]
				if want != got {
					return fmt.Errorf(
						"%s%s = %q, want %q from %s",
						prefix, key, got, want, res,
					)
				}
			}

			return nil
		}

		return fmt.Errorf(
			"%s does not list certificate %s", data, cert.Primary.ID,
		)
	}
}

func TestCertificatesDataSourceReadPaginates(t *testing.T) {
	pages := map[string]string{
		"1": `{
			"pagination": {"current_page": 1, "total_pages": 2, "total": 3,
				"per_page": 100, "large_set": false},
			"certificates": [
				{"id": "cert_a", "name": "a.example.com", "issuer": "self_signed",
				 "state": "issued", "expires_at": 1700086400,
				 "last_issued_at": 1700000000},
				{"id": "cert_b", "name": "b.example.com", "issuer": "lets_encrypt",
				 "state": "pending", "expires_at": null, "last_issued_at": null}
			]
		}`,
		"2": `{
			"pagination": {"current_page": 2, "total_pages": 2, "total": 3,
				"per_page": 100, "large_set": false},
			"certificates": [
				{"id": "cert_c", "name": "c.example.com", "issuer": "custom",
				 "state": "issued", "expires_at": 1700086400,
				 "last_issued_at": null}
			]
		}`,
	}

	t.Run("aggregates every page", func(t *testing.T) {
		var requests atomic.Int32
		meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
			func(w http.ResponseWriter, req *http.Request) {
				requests.Add(1)
				require.Equal(t,
					"/core/v1/organizations/organization/certificates",
					req.URL.Path)
				query := req.URL.Query()
				require.Equal(t, "test-organization",
					query.Get("organization[sub_domain]"))
				require.Equal(t, "100", query.Get("per_page"))
				body, ok := pages[query.Get("page")]
				require.Truef(t, ok, "unexpected page %q", query.Get("page"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
			},
		))

		state := readCertificatesDataSource(t, meta)
		require.False(t, state.Diagnostics.HasError(), state.Diagnostics)
		require.Equal(t, int32(2), requests.Load())

		var data CertificatesDataSourceModel
		require.False(t, state.State.Get(context.Background(), &data).HasError())
		items := data.Certificates.Elements()
		require.Len(t, items, 3)

		got := make([]map[string]string, 0, len(items))
		for _, item := range items {
			object, ok := item.(types.Object)
			require.True(t, ok)
			row := map[string]string{}
			for key, value := range object.Attributes() {
				str, ok := value.(types.String)
				require.True(t, ok)
				if str.IsNull() {
					row[key] = "<null>"
				} else {
					row[key] = str.ValueString()
				}
			}
			got = append(got, row)
		}
		require.Equal(t, []map[string]string{
			{
				"id": "cert_a", "name": "a.example.com", "issuer": "self_signed",
				"state": "issued", "expires_at": "2023-11-15T22:13:20Z",
				"last_issued_at": "2023-11-14T22:13:20Z",
			},
			{
				"id": "cert_b", "name": "b.example.com", "issuer": "lets_encrypt",
				"state": "pending", "expires_at": "<null>",
				"last_issued_at": "<null>",
			},
			{
				"id": "cert_c", "name": "c.example.com", "issuer": "custom",
				"state": "issued", "expires_at": "2023-11-15T22:13:20Z",
				"last_issued_at": "<null>",
			},
		}, got)
	})

	t.Run("second page error fails the read", func(t *testing.T) {
		meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
			func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if req.URL.Query().Get("page") == "1" {
					_, _ = w.Write([]byte(pages["1"]))
					return
				}
				// 403 is not retried by the HTTP client, unlike 5xx.
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error": {"code": "permission_denied",
					"description": "page two exploded"}}`))
			},
		))

		state := readCertificatesDataSource(t, meta)
		require.True(t, state.Diagnostics.HasError())
		requireDiagnosticContains(t, state.Diagnostics, "Certificates Error")
		requireDiagnosticContains(t, state.Diagnostics, "page two exploded")
	})
}

func readCertificatesDataSource(
	t *testing.T,
	meta *Meta,
) datasource.ReadResponse {
	t.Helper()
	ctx := context.Background()

	d := &CertificatesDataSource{M: meta}
	var schemaResp datasource.SchemaResponse
	d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
	require.False(t, schemaResp.Diagnostics.HasError())

	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	require.False(t, plan.Set(ctx, CertificatesDataSourceModel{
		Certificates: types.ListNull(certificatesDataSourceItemType),
	}).HasError())

	resp := datasource.ReadResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: plan.Raw},
	}
	d.Read(ctx, datasource.ReadRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: plan.Raw},
	}, &resp)

	return resp
}
