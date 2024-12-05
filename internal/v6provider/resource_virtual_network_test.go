package v6provider

import (
	"context"
	"fmt"
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

		m.Logger.Info("deleting virtual network", "id", vNet.Id, "name", vNet.Name)
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
						tt, "katapult_virtual_network.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_network.main", "name", name,
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

//
// Helpers
//

func testAccCheckKatapultVirtualNetworkAttrs(
	tt *testTools,
	res string, //nolint:unparam
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

		return resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr(res, "id", *vNet.Id),
			resource.TestCheckResourceAttr(res, "name", *vNet.Name),
			resource.TestCheckResourceAttr(
				res, "data_center_id", *vNet.DataCenter.Id,
			),
		)(s)
	}
}

func testAccCheckKatapultVirtualNetworkExists(
	tt *testTools,
	name string,
) resource.TestCheckFunc {
	m := tt.Meta

	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("virtual network not found: %s", name)
		}

		id := rs.Primary.ID

		_, err := m.Core.GetVirtualNetworkWithResponse(tt.Ctx,
			&core.GetVirtualNetworkParams{
				VirtualNetworkId: &id,
			},
		)
		if err != nil {
			return err
		}

		return nil
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
