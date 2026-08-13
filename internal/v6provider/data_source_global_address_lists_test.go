package v6provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceGlobalAddressLists_minimal(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultAddressListDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					data "katapult_global_address_lists" "global" {}
				`,
				),

				// Check that the known global address lists are present.
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckTypeSetElemNestedAttrs(
						"data.katapult_global_address_lists.global",
						"address_lists.*",
						map[string]string{
							"name": "Cloudflare",
						},
					),
					resource.TestCheckTypeSetElemNestedAttrs(
						"data.katapult_global_address_lists.global",
						"address_lists.*",
						map[string]string{
							"name": "BunnyCDN",
						},
					),
					resource.TestCheckTypeSetElemNestedAttrs(
						"data.katapult_global_address_lists.global",
						"address_lists.*",
						map[string]string{
							"name": "Amazon Cloudfront",
						},
					),
				),
			},
		},
	})
}
