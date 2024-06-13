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

				// Expect 3 responses and we check the 2nd and 3rd entries.
				// This is because the terraform-acc-test org has an
				// existing `Public DNS Servers` address list.
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.katapult_global_address_lists.global",
						"address_lists.#",
						"9",
					),
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
