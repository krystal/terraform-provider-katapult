package v6provider

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
	core "github.com/krystal/go-katapult/next/core"
)

func init() { //nolint:gochecknoinits
	resource.AddTestSweepers("katapult_address_list", &resource.Sweeper{
		Name: "katapult_address_list",
		F:    testSweepAddressLists,
	})
}

func testSweepAddressLists(_ string) error {
	m := sweepMeta()
	ctx := context.TODO()

	var addressLists []core.GetOrganizationAddressLists200ResponseAddressLists
	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		res, err := m.Core.GetOrganizationAddressListsWithResponse(ctx,
			&core.GetOrganizationAddressListsParams{
				OrganizationId: &m.confOrganization,
				Page:           &pageNum,
			})
		if err != nil {
			return err
		}

		resp := res.JSON200

		totalPages = *resp.Pagination.TotalPages
		addressLists = append(addressLists, resp.AddressLists...)
	}

	for _, list := range addressLists {
		if !strings.HasPrefix(*list.Name, testAccResourceNamePrefix) {
			continue
		}

		m.Logger.Info("deleting address list", "id", list.Id, "name", list.Name)
		_, err := m.Core.DeleteAddressListWithResponse(ctx,
			core.DeleteAddressListJSONRequestBody{
				AddressList: core.AddressListLookup{
					Id: list.Id,
				},
			})
		if err != nil {
			return err
		}
	}

	return nil
}

func TestAccKatapultAddressList_minimal(t *testing.T) {
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
				`, name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckKatapultAddressListExists(
						tt, "katapult_address_list.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_address_list.main", "name", name,
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

func TestAccKatapultAddressList_update(t *testing.T) {
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
				`, name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckKatapultAddressListExists(
						tt, "katapult_address_list.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_address_list.main", "name", name,
					),
				),
			},

			{
				Config: undent.Stringf(`
					resource "katapult_address_list" "main" {
					  name = "%s"
					}
				`, name+"-updated"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckKatapultAddressListExists(
						tt, "katapult_address_list.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_address_list.main", "name", name+"-updated",
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

func testAccCheckKatapultAddressListExists(
	tt *testTools,
	name string,
) resource.TestCheckFunc {
	m := tt.Meta

	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("address list not found: %s", name)
		}

		id := rs.Primary.ID

		_, err := m.Core.GetAddressListWithResponse(tt.Ctx,
			&core.GetAddressListParams{
				AddressListId: &id,
			})
		if err != nil {
			return err
		}

		return nil
	}
}

func testAccCheckKatapultAddressListDestroy(
	tt *testTools,
) resource.TestCheckFunc {
	m := tt.Meta

	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "katapult_address_list" {
				continue
			}

			id := rs.Primary.ID

			resp, err := m.Core.GetAddressListWithResponse(tt.Ctx,
				&core.GetAddressListParams{
					AddressListId: &id,
				})
			if err == nil && resp.JSON404 == nil {
				return fmt.Errorf("address list %s still exists", id)
			}
		}

		return nil
	}
}
