package v6provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/next/core"
)

func TestAccKatapultDataSourceVirtualNetworks_all(t *testing.T) {
	tt := newTestTools(t)

	dcLon := getKatapultDataCenter(tt, "uk-lon-01")
	dcAms := getKatapultDataCenter(tt, "nl-ams-01")

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultIPDestroy(tt),
		Steps: []resource.TestStep{
			{
				// The use of `depends_on` here is to ensure that the resources
				// are created in a predictable order, as otherwise tests are
				// likely to fail when running in VCR replay mode.
				Config: undent.Stringf(`
					resource "katapult_virtual_network" "backbone" {
					  name = "%s-backbone"
					}

					resource "katapult_virtual_network" "fallback" {
					  depends_on = [katapult_virtual_network.backbone]
					  name = "%s-fallback"
					  data_center_id = "%s"
					}

					resource "katapult_virtual_network" "backbone-ams" {
					  depends_on = [katapult_virtual_network.fallback]
					  name = "%s-backbone-ams"
					  data_center_id = "%s"
					}

					data "katapult_virtual_networks" "all" {
					  depends_on = [katapult_virtual_network.backbone-ams]
					}
				`, name, name, *dcLon.Id, name, *dcAms.Id),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVirtualNetworksAttrs(tt,
						"data.katapult_virtual_networks.all",
					),
				),
			},
		},
	})
}

func testAccCheckKatapultVirtualNetworksAttrs(
	tt *testTools,
	res string,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		resp, err := tt.Meta.Core.GetOrganizationVirtualNetworksWithResponse(
			tt.Ctx, &core.GetOrganizationVirtualNetworksParams{
				OrganizationSubDomain: &tt.Meta.confOrganization,
			},
		)
		if err != nil {
			tt.T.Fatalf("error fetching list of networks: %s", err)
		}

		networks := resp.JSON200.VirtualNetworks

		checks := []resource.TestCheckFunc{}

		for _, network := range networks {
			attrs := map[string]string{
				"id":             *network.Id,
				"name":           *network.Name,
				"data_center_id": *network.DataCenter.Id,
			}

			checks = append(checks,
				resource.TestCheckTypeSetElemNestedAttrs(
					res, "virtual_networks.*", attrs,
				),
			)
		}

		return resource.ComposeAggregateTestCheckFunc(checks...)(s)
	}
}
