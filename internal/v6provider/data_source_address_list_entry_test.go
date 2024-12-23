package v6provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceAddressListEntry_minimal(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()
	entryOneName := name + "-entry-one"

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

					resource "katapult_address_list_entry" "cf" {
					 address_list_id = katapult_address_list.main.id
					 name            = "%s"
					 address         = "1.1.1.1"
					}
					`,
					name,
					entryOneName,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultAddressListExists(
						tt, "katapult_address_list.main",
					),
					testAccCheckKatapultAddressListEntryExists(
						tt, "katapult_address_list_entry.cf",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_address_list" "main" {
					 name = "%s"
					}

					resource "katapult_address_list_entry" "cf" {
					 address_list_id = katapult_address_list.main.id
					 name            = "%s"
					 address         = "1.1.1.1"
					}

					data "katapult_address_list_entry" "cf" {
					 id = katapult_address_list_entry.cf.id
					}
					`,
					name,
					entryOneName,
				),
				Check: resource.ComposeAggregateTestCheckFunc(

					resource.TestCheckResourceAttr(
						"data.katapult_address_list_entry.cf",
						"name",
						entryOneName,
					),
					resource.TestCheckResourceAttr(
						"data.katapult_address_list_entry.cf",
						"address",
						"1.1.1.1",
					),
				),
			},
		},
	})
}
