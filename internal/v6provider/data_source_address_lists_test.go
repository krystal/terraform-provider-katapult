package v6provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceAddressLists_minimal(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()
	altName := name + "-alt"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultAddressListDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_address_list" "main" {
					  name = "%s"
					}

					resource "katapult_address_list" "alt" {
					  name = "%s"
					  depends_on = [katapult_address_list.main]
					}

					`,
					name,
					altName,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultAddressListExists(
						tt, "katapult_address_list.main",
					),
					testAccCheckKatapultAddressListExists(
						tt, "katapult_address_list.alt",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_address_list" "main" {
					  name = "%s"
					}

					resource "katapult_address_list" "alt" {
					  name = "%s"
					  depends_on = [katapult_address_list.main]
					}

					data "katapult_address_lists" "lists" {}

				`,
					name,
					altName,
				),

				// Expect 3 responses and we check the 2nd and 3rd entries.
				// This is because the terraform-acc-test org has an
				// existing `Public DNS Servers` address list.
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.katapult_address_lists.lists",
						"address_lists.#",
						"3",
					),
					resource.TestCheckTypeSetElemNestedAttrs(
						"data.katapult_address_lists.lists",
						"address_lists.*",
						map[string]string{
							"name": name,
						},
					),
					resource.TestCheckTypeSetElemNestedAttrs(
						"data.katapult_address_lists.lists",
						"address_lists.*",
						map[string]string{
							"name": altName,
						},
					),
				),
			},
		},
	})
}
