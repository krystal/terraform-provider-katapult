package v6provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/krystal/go-katapult/next/core"
)

func TestAccKatapultDataSourceNetworks_all(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultIPDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: `data "katapult_networks" "all" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultNetworksAttrs(tt,
						"data.katapult_networks.all",
					),
					// Sanity checks against networks which are unlikely to
					// change.
					resource.TestCheckTypeSetElemNestedAttrs(
						"data.katapult_networks.all",
						"networks.*", map[string]string{
							"id":        "netw_gVRkZdSKczfNg34P",
							"permalink": "uk-lon-01-public",
						},
					),
					resource.TestCheckTypeSetElemNestedAttrs(
						"data.katapult_networks.all",
						"networks.*", map[string]string{
							"id":        "netw_gnlxBcgtF7xyMslK",
							"permalink": "eu-ams-01",
						},
					),
				),
			},
		},
	})
}

func testAccCheckKatapultNetworksAttrs(
	tt *testTools,
	res string,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		resp, err := tt.Meta.Core.GetOrganizationAvailableNetworksWithResponse(
			tt.Ctx, &core.GetOrganizationAvailableNetworksParams{
				OrganizationSubDomain: &tt.Meta.confOrganization,
			},
		)
		if err != nil {
			tt.T.Fatalf("error fetching list of networks: %s", err)
		}

		networks := resp.JSON200.Networks

		checks := []resource.TestCheckFunc{}

		for _, network := range networks {
			attrs := map[string]string{
				"id":             *network.Id,
				"name":           *network.Name,
				"data_center_id": *network.DataCenter.Id,
			}

			if !network.Permalink.IsNull() {
				permalink, err := network.Permalink.Get()
				if err != nil {
					tt.T.Fatalf("error fetching network permalink: %s", err)
				}

				attrs["permalink"] = permalink
			}

			checks = append(checks,
				resource.TestCheckTypeSetElemNestedAttrs(
					res, "networks.*", attrs,
				),
			)
		}

		return resource.ComposeAggregateTestCheckFunc(checks...)(s)
	}
}
