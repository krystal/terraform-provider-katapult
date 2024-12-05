package v6provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceAddressListEntries_minimal(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()
	entryOneName := name + "-entry-one"
	entryTwoName := name + "-entry-two"

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

					resource "katapult_address_list_entry" "goog" {
					depends_on = [katapult_address_list_entry.cf]
					 address_list_id = katapult_address_list.main.id
					 name            = "%s"
					 address         = "8.8.8.8"
					}
					`,
					name,
					entryOneName,
					entryTwoName,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultAddressListExists(
						tt, "katapult_address_list.main",
					),
					testAccCheckKatapultAddressListEntryExists(
						tt, "katapult_address_list_entry.cf",
					),
					testAccCheckKatapultAddressListEntryExists(
						tt, "katapult_address_list_entry.goog",
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

					resource "katapult_address_list_entry" "goog" {
					 depends_on = [katapult_address_list_entry.cf]
					 address_list_id = katapult_address_list.main.id
					 name            = "%s"
					 address         = "8.8.8.8"
					}

					data "katapult_address_list_entries" "entries" {
					 address_list_id = katapult_address_list.main.id
					}
					`,
					name,
					entryOneName,
					entryTwoName,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.katapult_address_list_entries.entries",
						"entries.#",
						"2",
					),
					resource.TestCheckTypeSetElemNestedAttrs(
						"data.katapult_address_list_entries.entries",
						"entries.*",
						map[string]string{
							"name":    entryOneName,
							"address": "1.1.1.1",
						},
					),
					resource.TestCheckTypeSetElemNestedAttrs(
						"data.katapult_address_list_entries.entries",
						"entries.*",
						map[string]string{
							"name":    entryTwoName,
							"address": "8.8.8.8",
						},
					),
				),
			},
		},
	})
}
