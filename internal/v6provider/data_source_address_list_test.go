package v6provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceAddressList_minimal(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

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
					`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultAddressListExists(
						tt, "katapult_address_list.main",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_address_list" "main" {
					  name = "%s"
					}

					data "katapult_address_list" "main" {
						id = katapult_address_list.main.id
					}
				`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.katapult_address_list.main",
						"name",
						name,
					),
				),
			},
		},
	})
}
