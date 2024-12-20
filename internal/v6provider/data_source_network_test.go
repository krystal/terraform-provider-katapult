package v6provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/next/core"
)

func TestAccKatapultDataSourceNetwork_by_id(t *testing.T) {
	tt := newTestTools(t)

	resp, err := tt.Meta.Core.GetDataCenterDefaultNetworkWithResponse(
		tt.Ctx, &core.GetDataCenterDefaultNetworkParams{
			DataCenterPermalink: &tt.Meta.confDataCenter,
		},
	)
	if err != nil {
		t.Fatalf("error fetching default network: %s", err)
	}

	network := resp.JSON200.Network

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultIPDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					data "katapult_network" "my_net" {
					  id = "%s"
					}`,
					*network.Id,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultNetworkAttrs(tt,
						"data.katapult_network.my_net",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_network.my_net", "name", *network.Name,
					),
					resource.TestCheckResourceAttr(
						"data.katapult_network.my_net", "default", "true",
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceNetwork_by_permalink(t *testing.T) {
	tt := newTestTools(t)

	resp, err := tt.Meta.Core.GetDataCenterDefaultNetworkWithResponse(
		tt.Ctx, &core.GetDataCenterDefaultNetworkParams{
			DataCenterPermalink: &tt.Meta.confDataCenter,
		},
	)
	if err != nil {
		t.Fatalf("error fetching default network: %s", err)
	}

	network := resp.JSON200.Network
	permalink, err := network.Permalink.Get()
	if err != nil {
		t.Fatalf("error reading network permalink: %s", err)
	}

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultIPDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					data "katapult_network" "my_net" {
					  permalink = "%s"
					}`,
					permalink,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultNetworkAttrs(tt,
						"data.katapult_network.my_net",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_network.my_net", "name", *network.Name,
					),
					resource.TestCheckResourceAttr(
						"data.katapult_network.my_net", "default", "true",
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceNetwork_default(t *testing.T) {
	tt := newTestTools(t)

	resp, err := tt.Meta.Core.GetDataCenterDefaultNetworkWithResponse(
		tt.Ctx, &core.GetDataCenterDefaultNetworkParams{
			DataCenterPermalink: &tt.Meta.confDataCenter,
		},
	)
	if err != nil {
		t.Fatalf("error fetching default network: %s", err)
	}

	network := resp.JSON200.Network

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultIPDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: `data "katapult_network" "my_net" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultNetworkAttrs(tt,
						"data.katapult_network.my_net",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_network.my_net", "name", *network.Name,
					),
					resource.TestCheckResourceAttr(
						"data.katapult_network.my_net", "default", "true",
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceNetwork_default_other_data_center(t *testing.T) {
	tt := newTestTools(t)

	// "Public Network - AMS (eu-ams-01)
	dc := getKatapultDataCenter(tt, "nl-ams-01")
	netResp, err := tt.Meta.Core.GetDataCenterDefaultNetworkWithResponse(
		tt.Ctx, &core.GetDataCenterDefaultNetworkParams{
			DataCenterId: dc.Id,
		},
	)
	if err != nil {
		t.Fatalf("error fetching default network: %s", err)
	}

	network := netResp.JSON200.Network

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultIPDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					data "katapult_network" "my_net" {
					  data_center_id = "%s"
					}`,
					*dc.Id,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultNetworkAttrs(tt,
						"data.katapult_network.my_net",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_network.my_net", "name", *network.Name,
					),
					resource.TestCheckResourceAttr(
						"data.katapult_network.my_net", "default", "true",
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceNetwork_other_by_id(t *testing.T) {
	tt := newTestTools(t)

	// "Public Network - AMS (eu-ams-01)
	dc := getKatapultDataCenter(tt, "nl-ams-01")
	netResp, err := tt.Meta.Core.GetDataCenterDefaultNetworkWithResponse(
		tt.Ctx, &core.GetDataCenterDefaultNetworkParams{
			DataCenterId: dc.Id,
		},
	)
	if err != nil {
		t.Fatalf("error fetching default network: %s", err)
	}

	network := netResp.JSON200.Network

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultIPDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					data "katapult_network" "my_net" {
					  id = "%s"
					}`,
					*network.Id,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultNetworkAttrs(tt,
						"data.katapult_network.my_net",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_network.my_net", "name", *network.Name,
					),
					resource.TestCheckResourceAttr(
						"data.katapult_network.my_net",
						"permalink", "eu-ams-01",
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceNetwork_other_by_permalink(t *testing.T) {
	tt := newTestTools(t)

	// "Public Network - AMS (eu-ams-01)
	dc := getKatapultDataCenter(tt, "nl-ams-01")
	netResp, err := tt.Meta.Core.GetDataCenterDefaultNetworkWithResponse(
		tt.Ctx, &core.GetDataCenterDefaultNetworkParams{
			DataCenterId: dc.Id,
		},
	)
	if err != nil {
		t.Fatalf("error fetching default network: %s", err)
	}

	network := netResp.JSON200.Network
	permalink, err := network.Permalink.Get()
	if err != nil {
		t.Fatalf("error reading network permalink: %s", err)
	}

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultIPDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					data "katapult_network" "my_net" {
					  permalink = "%s"
					}`,
					permalink,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultNetworkAttrs(tt,
						"data.katapult_network.my_net",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_network.my_net", "name", *network.Name,
					),
					resource.TestCheckResourceAttr(
						"data.katapult_network.my_net",
						"permalink", "eu-ams-01",
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceNetwork_invalid_attributes(t *testing.T) {
	tt := newTestTools(t)

	dc := getKatapultDataCenter(tt, tt.Meta.confDataCenter)
	resp, err := tt.Meta.Core.GetDataCenterDefaultNetworkWithResponse(
		tt.Ctx, &core.GetDataCenterDefaultNetworkParams{
			DataCenterPermalink: &tt.Meta.confDataCenter,
		},
	)
	if err != nil {
		t.Fatalf("error fetching default network: %s", err)
	}

	network := resp.JSON200.Network
	permalink, err := network.Permalink.Get()
	if err != nil {
		t.Fatalf("error reading network permalink: %s", err)
	}

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultIPDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					data "katapult_network" "my_net" {
					  id = "%s"
					  permalink = "%s"
					}`,
					*network.Id, permalink,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						`Attribute "permalink" cannot be specified when ` +
							`"id" is specified`,
					),
				),
			},
			{
				Config: undent.Stringf(`
					data "katapult_network" "my_net" {
					  id = "%s"
					  data_center_id = "%s"
					}`,
					*network.Id, *dc.Id,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						`Attribute "data_center_id" cannot be specified when ` +
							`"id" is specified`,
					),
				),
			},
			{
				Config: undent.Stringf(`
					data "katapult_network" "my_net" {
					  permalink = "%s"
					  data_center_id = "%s"
					}`,
					permalink, *dc.Id,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						`Attribute "data_center_id" cannot be specified when ` +
							`"permalink" is specified`,
					),
				),
			},
		},
	})
}

func testAccCheckKatapultNetworkAttrs(
	tt *testTools,
	res string, //nolint:unparam
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("resource not found: %s", res)
		}

		response, err := tt.Meta.Core.GetNetworkWithResponse(tt.Ctx,
			&core.GetNetworkParams{
				NetworkId: &rs.Primary.ID,
			})
		if err != nil {
			return err
		}

		network := response.JSON200.Network
		if network.Id == nil {
			return fmt.Errorf("Network not found: %s", rs.Primary.ID)
		}

		permalink, err := network.Permalink.Get()
		if err != nil {
			return fmt.Errorf("Network permalink read error: %w", err)
		}

		defaultResp, err := tt.Meta.Core.
			GetDataCenterDefaultNetworkWithResponse(
				tt.Ctx, &core.GetDataCenterDefaultNetworkParams{
					DataCenterId: network.DataCenter.Id,
				},
			)
		if err != nil {
			return err
		}

		defaultID := *defaultResp.JSON200.Network.Id

		isDefault := "false"
		if *network.Id == defaultID {
			isDefault = "true"
		}

		return resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr(res, "id", *network.Id),
			resource.TestCheckResourceAttr(res, "name", *network.Name),
			resource.TestCheckResourceAttr(res, "permalink", permalink),
			resource.TestCheckResourceAttr(
				res, "data_center_id", *network.DataCenter.Id,
			),
			resource.TestCheckResourceAttr(res, "default", isDefault),
		)(s)
	}
}
