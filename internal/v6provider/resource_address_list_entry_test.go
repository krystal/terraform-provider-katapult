package v6provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
	core "github.com/krystal/go-katapult/next/core"
)

func TestAccKatapultAddressListEntry_minimal(t *testing.T) {
	tt := newTestTools(t)
	name := tt.ResourceName()
	listName := name + "-list"
	entryName := name + "-entry"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultAddressListEntryDestroy(tt), //nolint:lll // helper
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_address_list" "main" {
					 name = "%s"
					}

					resource "katapult_address_list_entry" "main" {
					 address_list_id = katapult_address_list.main.id
					 name            = "%s"
					 address         = "1.1.1.1"
					}
				`, listName, entryName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckKatapultAddressListEntryExists(
						tt, "katapult_address_list_entry.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_address_list_entry.main", "name", entryName,
					),
					resource.TestCheckResourceAttr(
						"katapult_address_list_entry.main",
						"address",
						"1.1.1.1",
					),
				),
			},
			{
				ResourceName:      "katapult_address_list.main",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultAddressListEntry_update(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()
	listName := name + "-list"
	entryName := name + "-entry"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultAddressListEntryDestroy(tt), //nolint:lll // helper
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_address_list" "main" {
					 name = "%s"
					}

					resource "katapult_address_list_entry" "main" {
					 address_list_id = katapult_address_list.main.id
					 name            = "%s"
					 address         = "1.1.1.1"
					}
				`, listName, entryName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckKatapultAddressListEntryExists(
						tt, "katapult_address_list_entry.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_address_list_entry.main",
						"name",
						entryName,
					),
					resource.TestCheckResourceAttr(
						"katapult_address_list_entry.main",
						"address",
						"1.1.1.1",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_address_list" "main" {
					 name = "%s"
					}

					resource "katapult_address_list_entry" "main" {
					 address_list_id = katapult_address_list.main.id
					 name            = "%s"
					 address         = "8.8.8.8"
					}
				`, listName, entryName+"-updated"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckKatapultAddressListEntryExists(
						tt, "katapult_address_list_entry.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_address_list_entry.main",
						"name",
						entryName+"-updated",
					),
					resource.TestCheckResourceAttr(
						"katapult_address_list_entry.main",
						"address",
						"8.8.8.8",
					),
				),
			},
			{
				ResourceName:      "katapult_address_list.main",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

//
// Helpers
//

func testAccCheckKatapultAddressListEntryExists(
	tt *testTools,
	name string,
) resource.TestCheckFunc {
	m := tt.Meta

	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("address list entry not found: %s", name)
		}

		id := rs.Primary.ID

		_, err := m.Core.GetAddressListEntryWithResponse(tt.Ctx,
			&core.GetAddressListEntryParams{
				AddressListEntryId: &id,
			})
		if err != nil {
			return err
		}

		return nil
	}
}

func testAccCheckKatapultAddressListEntryDestroy(
	tt *testTools,
) resource.TestCheckFunc {
	m := tt.Meta

	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "katapult_address_list_entry" {
				continue
			}

			id := rs.Primary.ID

			resp, err := m.Core.GetAddressListEntryWithResponse(tt.Ctx,
				&core.GetAddressListEntryParams{
					AddressListEntryId: &id,
				})
			if err == nil && resp.JSON404 == nil {
				return fmt.Errorf("address list entry %s still exists", id)
			}
		}

		return nil
	}
}
