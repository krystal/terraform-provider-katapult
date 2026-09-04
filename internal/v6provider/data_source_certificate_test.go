package v6provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceCertificate_minimal(t *testing.T) {
	tt := newTestTools(t)
	name := testAccCertificateHostname(tt)
	res := "katapult_self_signed_certificate.web"
	data := "data.katapult_certificate.web"

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

					data "katapult_certificate" "web" {
					  id = katapult_self_signed_certificate.web.id
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultCertificateAttrs(tt, data),
					resource.TestCheckResourceAttrPair(data, "id", res, "id"),
					resource.TestCheckResourceAttrPair(data, "name", res, "name"),
					resource.TestCheckResourceAttrPair(data, "issuer", res, "issuer"),
					resource.TestCheckResourceAttrPair(data, "state", res, "state"),
					resource.TestCheckResourceAttrPair(
						data, "certificate", res, "certificate",
					),
					resource.TestCheckResourceAttrPair(
						data, "private_key", res, "private_key",
					),
					resource.TestCheckResourceAttrPair(
						data, "created_at", res, "created_at",
					),
					resource.TestCheckResourceAttrPair(
						data, "expires_at", res, "expires_at",
					),
					resource.TestCheckResourceAttr(data, "additional_names.#", "0"),
					resource.TestCheckNoResourceAttr(data, "authorization_method"),
					resource.TestCheckNoResourceAttr(data, "chain"),
					resource.TestCheckNoResourceAttr(data, "issue_error"),
				),
			},
		},
	})
}
