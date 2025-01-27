package v6provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/next/core"
)

func init() { //nolint:gochecknoinits
	resource.AddTestSweepers("katapult_virtual_network", &resource.Sweeper{
		Name: "katapult_virtual_network",
		F:    testSweepVirtualNetworks,
	})
}

func testSweepVirtualNetworks(_ string) error {
	m := sweepMeta()
	ctx := context.TODO()

	var virtualNetwork []core.VirtualNetwork
	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		res, err := m.Core.GetOrganizationVirtualNetworksWithResponse(ctx,
			&core.GetOrganizationVirtualNetworksParams{
				OrganizationId: &m.confOrganization,
				Page:           &pageNum,
			})
		if err != nil {
			return err
		}

		resp := res.JSON200

		totalPages, _ = resp.Pagination.TotalPages.Get()
		virtualNetwork = append(virtualNetwork, resp.VirtualNetworks...)
	}

	for _, vNet := range virtualNetwork {
		if !strings.HasPrefix(*vNet.Name, testAccResourceNamePrefix) {
			continue
		}

		m.Logger.Info("deleting virtual network",
			"id", vNet.Id,
			"name", vNet.Name,
		)
		_, err := m.Core.DeleteVirtualNetworkWithResponse(ctx,
			core.DeleteVirtualNetworkJSONRequestBody{
				VirtualNetwork: core.VirtualNetworkLookup{Id: vNet.Id},
			},
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func TestAccKatapultVirtualNetwork_minimal(t *testing.T) {
	tt := newTestTools(t)

	dc := getKatapultDataCenter(tt, tt.Meta.confDataCenter)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultVirtualNetworkDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_virtual_network" "main" {
					  name = "%s"
					}
				`, name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckKatapultVirtualNetworkAttrs(
						tt, "katapult_virtual_network.main", nil,
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_network.main", "name", name,
					),
					// Ensure default data center is used.
					resource.TestCheckResourceAttr(
						"katapult_virtual_network.main",
						"data_center_id", *dc.Id,
					),
				),
			},
			{
				ResourceName:      "katapult_virtual_network.main",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultVirtualNetwork_update_name(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	var createID, updateID string

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultVirtualNetworkDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_virtual_network" "main" {
					  name = "%s"
					}
				`, name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckKatapultVirtualNetworkAttrs(
						tt, "katapult_virtual_network.main", &createID,
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_network.main", "name", name,
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_virtual_network" "main" {
					  name = "%s"
					}
				`, name+"-other"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckKatapultVirtualNetworkAttrs(
						tt, "katapult_virtual_network.main", &updateID,
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_network.main", "name", name+"-other",
					),
					testAccCheckResourceAttrNotChanged(
						"katapult_virtual_network.main", "id",
						&createID, &updateID,
					),
				),
			},
			{
				ResourceName:      "katapult_virtual_network.main",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultVirtualNetwork_update_data_center_id(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	dcLon := getKatapultDataCenter(tt, "uk-lon-01")
	dcAms := getKatapultDataCenter(tt, "nl-ams-01")

	var createID, updateID string

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultVirtualNetworkDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_virtual_network" "fallback" {
					  name = "%s"
					  data_center_id = "%s"
					}
				`, name, *dcLon.Id),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckKatapultVirtualNetworkAttrs(
						tt, "katapult_virtual_network.fallback", &createID,
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_network.fallback",
						"data_center_id", *dcLon.Id,
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_virtual_network" "fallback" {
					  name = "%s"
					  data_center_id = "%s"
					}
				`, name, *dcAms.Id),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckKatapultVirtualNetworkAttrs(
						tt, "katapult_virtual_network.fallback", &updateID,
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_network.fallback",
						"data_center_id", *dcAms.Id,
					),
					testAccCheckResourceAttrChanged(
						"katapult_virtual_network.fallback", "id",
						&createID, &updateID,
					),
				),
			},
			{
				ResourceName:      "katapult_virtual_network.fallback",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultVirtualNetwork_empty_block(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultVirtualNetworkDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: `resource "katapult_virtual_network" "main" {}`,
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						`The argument "name" is required`,
					),
				),
			},
		},
	})
}

func TestAccKatapultVirtualNetwork_empty_name(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultVirtualNetworkDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					resource "katapult_virtual_network" "main" {
					  name = ""
					}
				`),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						`Attribute name string cannot be empty`,
					),
				),
			},
		},
	})
}

//
// Helpers
//

func testAccCheckKatapultVirtualNetworkAttrs(
	tt *testTools,
	res string,
	id *string,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("resource not found: %s", res)
		}

		response, err := tt.Meta.Core.GetVirtualNetworkWithResponse(tt.Ctx,
			&core.GetVirtualNetworkParams{
				VirtualNetworkId: &rs.Primary.ID,
			})
		if err != nil {
			return err
		}

		vNet := response.JSON200.VirtualNetwork
		if vNet.Id == nil {
			return fmt.Errorf("Virtual network not found: %s", rs.Primary.ID)
		}

		// Expose ID if provided by caller.
		if id != nil {
			*id = *vNet.Id
		}

		return resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr(res, "id", *vNet.Id),
			resource.TestCheckResourceAttr(res, "name", *vNet.Name),
			resource.TestCheckResourceAttr(
				res, "data_center_id", *vNet.DataCenter.Id,
			),
		)(s)
	}
}

func testAccCheckKatapultVirtualNetworkDestroy(
	tt *testTools,
) resource.TestCheckFunc {
	m := tt.Meta

	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "katapult_virtual_network" {
				continue
			}

			id := rs.Primary.ID

			resp, err := m.Core.GetVirtualNetworkWithResponse(tt.Ctx,
				&core.GetVirtualNetworkParams{
					VirtualNetworkId: &id,
				},
			)
			if err == nil && resp.JSON404 == nil {
				return fmt.Errorf("virtual network %s still exists", id)
			}
		}

		return nil
	}
}
