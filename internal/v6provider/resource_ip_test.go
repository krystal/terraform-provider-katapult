package v6provider

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
	core "github.com/krystal/go-katapult/next/core"

	"github.com/stretchr/testify/require"
)

func init() { //nolint:gochecknoinits
	resource.AddTestSweepers("katapult_ip", &resource.Sweeper{
		Name:         "katapult_ip",
		F:            testSweepIPs,
		Dependencies: []string{"katapult_load_balancer"},
	})
}

func testSweepIPs(_ string) error {
	m := sweepMeta()
	ctx := context.TODO()

	var ips []core.GetOrganizationIPAddresses200ResponseIPAddresses
	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		res, err := m.Core.GetOrganizationIpAddressesWithResponse(ctx,
			&core.GetOrganizationIpAddressesParams{
				OrganizationSubDomain: &m.confOrganization,
				Page:                  &pageNum,
			})
		if err != nil {
			return err
		}

		totalPages = res.JSON200.Pagination.TotalPages.MustGet()
		ips = append(ips, res.JSON200.IpAddresses...)
	}

	for _, ip := range ips {
		if !ip.AllocationId.IsNull() && ip.AllocationType.IsSpecified() {
			m.Logger.Info(
				"skipping IP address: has allocation",
				"id", ip.Id,
				"address", ip.Address,
				"allocation_id", ip.AllocationId,
				"allocation_type", ip.AllocationType,
			)

			continue
		}

		m.Logger.Info("deleting IP address", "id", ip.Id, "address", ip.Address)

		_, err := m.Core.DeleteIpAddressWithResponse(ctx,
			core.DeleteIpAddressJSONRequestBody{
				IpAddress: core.IPAddressLookup{
					Id: ip.Id,
				},
			})
		if err != nil {
			return err
		}
	}

	return nil
}

func TestAccKatapultIP_minimal(t *testing.T) {
	tt := newTestTools(t)

	res, err := tt.Meta.Core.GetDataCenterDefaultNetworkWithResponse(tt.Ctx,
		&core.GetDataCenterDefaultNetworkParams{
			DataCenterPermalink: &tt.Meta.confDataCenter,
		})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotNil(t, res.JSON200)

	network := res.JSON200.Network
	require.NotNil(t, network.Id)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,

		CheckDestroy: testAccCheckKatapultIPDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: `resource "katapult_ip" "web" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultIPAttrs(tt, "katapult_ip.web"),
					resource.TestCheckResourceAttr(
						"katapult_ip.web", "network_id", *network.Id,
					),
					resource.TestMatchResourceAttr(
						"katapult_ip.web",
						"address", regexp.MustCompile(
							`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$`,
						),
					),
					resource.TestCheckResourceAttr(
						"katapult_ip.web", "version", "4",
					),
					resource.TestCheckResourceAttr(
						"katapult_ip.web", "vip", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_ip.web", "label", "",
					),
				),
			},
			{
				ResourceName:      "katapult_ip.web",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultIP_ipv4(t *testing.T) {
	tt := newTestTools(t)

	res, err := tt.Meta.Core.GetDataCenterDefaultNetworkWithResponse(tt.Ctx,
		&core.GetDataCenterDefaultNetworkParams{
			DataCenterPermalink: &tt.Meta.confDataCenter,
		})
	require.NoError(t, err)

	network := res.JSON200.Network
	require.NotNil(t, network.Id)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultIPDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					resource "katapult_ip" "web" {
						version = 4
					}`,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultIPAttrs(tt, "katapult_ip.web"),
					resource.TestCheckResourceAttr(
						"katapult_ip.web", "network_id", *network.Id,
					),
					resource.TestMatchResourceAttr(
						"katapult_ip.web",
						"address", regexp.MustCompile(
							`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$`,
						),
					),
					resource.TestCheckResourceAttr(
						"katapult_ip.web", "version", "4",
					),
					resource.TestCheckResourceAttr(
						"katapult_ip.web", "vip", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_ip.web", "label", "",
					),
				),
			},
			{
				ResourceName:      "katapult_ip.web",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultIP_ipv6(t *testing.T) {
	tt := newTestTools(t)

	res, err := tt.Meta.Core.GetDataCenterDefaultNetworkWithResponse(tt.Ctx,
		&core.GetDataCenterDefaultNetworkParams{
			DataCenterPermalink: &tt.Meta.confDataCenter,
		})
	require.NoError(t, err)

	network := res.JSON200.Network
	require.NotNil(t, network.Id)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultIPDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					resource "katapult_ip" "web" {
						version = 6
					}`,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultIPAttrs(tt, "katapult_ip.web"),
					resource.TestCheckResourceAttr(
						"katapult_ip.web", "network_id", *network.Id,
					),
					resource.TestMatchResourceAttr(
						"katapult_ip.web",
						"address", regexp.MustCompile(`:.*:`),
					),
					resource.TestCheckResourceAttr(
						"katapult_ip.web", "version", "6",
					),
					resource.TestCheckResourceAttr(
						"katapult_ip.web", "vip", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_ip.web", "label", "",
					),
				),
			},
			{
				ResourceName:      "katapult_ip.web",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultIP_ipv5(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultIPDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					resource "katapult_ip" "web" {
						version = 5
					}`,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						`Attribute version value must be one ` +
							`of: ["4" "6"], got: 5`,
					),
				),
			},
		},
	})
}

func TestAccKatapultIP_vip(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultIPDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_ip" "vip" {
					  vip = true
					  label = "%s"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultIPAttrs(tt, "katapult_ip.vip"),
					resource.TestCheckResourceAttr(
						"katapult_ip.vip", "vip", "true",
					),
					resource.TestCheckResourceAttr(
						"katapult_ip.vip", "label", name,
					),
				),
			},
			{
				ResourceName:      "katapult_ip.vip",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultIP_vip_empty_label(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultIPDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					resource "katapult_ip" "vip" {
					  vip = true
					  label = ""
					}`,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						`Attribute label string length must be at least 1,` +
							` got: 0`,
					),
				),
			},
		},
	})
}

func TestAccKatapultIP_vip_without_label(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultIPDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					resource "katapult_ip" "vip" {
					  vip = true
					}`,
				),
				ExpectError: regexp.MustCompile(
					`(?s).*validation_error.+Label can't be blank.*`,
				),
			},
		},
	})
}

func TestAccKatapultIP_label_without_vip(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultIPDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					resource "katapult_ip" "vip" {
					  label = "hello"
					}`,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						`Attribute "vip" must be specified when ` +
							`"label" is specified`,
					),
				),
			},
		},
	})
}

func TestAccKatapultIP_with_network_id(t *testing.T) {
	tt := newTestTools(t)

	res, err := tt.Meta.Core.GetDataCenterDefaultNetworkWithResponse(tt.Ctx,
		&core.GetDataCenterDefaultNetworkParams{
			DataCenterPermalink: &tt.Meta.confDataCenter,
		})
	require.NoError(t, err)

	network := res.JSON200.Network
	require.NotNil(t, network.Id)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultIPDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_ip" "net" {
					  network_id = "%s"
					}`,
					*network.Id,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultIPAttrs(tt, "katapult_ip.net"),
					resource.TestCheckResourceAttr(
						"katapult_ip.net", "network_id", *network.Id,
					),
				),
			},
			{
				ResourceName:      "katapult_ip.net",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultIP_update(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultIPDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: `resource "katapult_ip" "update" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultIPAttrs(tt, "katapult_ip.update"),
					resource.TestCheckResourceAttr(
						"katapult_ip.update", "vip", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_ip.update", "label", "",
					),
				),
			},
			{
				Config: undent.String(`
					resource "katapult_ip" "update" {
						vip = true
						label = "vip-yes"
					}`,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultIPAttrs(tt, "katapult_ip.update"),
					resource.TestCheckResourceAttr(
						"katapult_ip.update", "vip", "true",
					),
					resource.TestCheckResourceAttr(
						"katapult_ip.update", "label", "vip-yes",
					),
				),
			},
			{
				Config: undent.String(`
					resource "katapult_ip" "update" {
						vip = true
						label = "vip-oh-yes"
					}`,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultIPAttrs(tt, "katapult_ip.update"),
					resource.TestCheckResourceAttr(
						"katapult_ip.update", "vip", "true",
					),
					resource.TestCheckResourceAttr(
						"katapult_ip.update", "label", "vip-oh-yes",
					),
				),
			},
			{
				Config: undent.String(`
					resource "katapult_ip" "update" {
						vip = false
					}`,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultIPAttrs(tt, "katapult_ip.update"),
					resource.TestCheckResourceAttr(
						"katapult_ip.update", "vip", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_ip.update", "label", "",
					),
				),
			},
		},
	})
}

//
// Helpers
//

func testAccCheckKatapultIPExists(
	tt *testTools,
	res string,
) resource.TestCheckFunc {
	m := tt.Meta

	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("resource not found: %s", res)
		}

		response, err := m.Core.GetIpAddressWithResponse(tt.Ctx,
			&core.GetIpAddressParams{
				IpAddressId: &rs.Primary.ID,
			})
		if err != nil {
			return err
		}

		ip := response.JSON200.IpAddress
		if ip.Id == nil {
			return fmt.Errorf("IP address not found: %s", rs.Primary.ID)
		}

		return resource.TestCheckResourceAttr(res, "id", *ip.Id)(s)
	}
}

func testAccCheckKatapultIPAttrs(
	tt *testTools,
	res string,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("resource not found: %s", res)
		}

		response, err := tt.Meta.Core.GetIpAddressWithResponse(tt.Ctx,
			&core.GetIpAddressParams{
				IpAddressId: &rs.Primary.ID,
			})
		if err != nil {
			return err
		}

		ip := response.JSON200.IpAddress
		if ip.Id == nil {
			return fmt.Errorf("IP address not found: %s", rs.Primary.ID)
		}

		allocationType, _ := ip.AllocationType.Get()
		allocationID, _ := ip.AllocationId.Get()

		tfs := []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(res, "id", *ip.Id),
			resource.TestCheckResourceAttr(res, "address", *ip.Address),
			resource.TestCheckResourceAttr(
				res, "address_with_mask", *ip.AddressWithMask,
			),
			resource.TestCheckResourceAttr(res, "reverse_dns", *ip.ReverseDns),
			resource.TestCheckResourceAttr(
				res,
				"version",
				strconv.FormatInt(flattenIPVersion(*ip.Address), 10),
			),
			resource.TestCheckResourceAttr(
				res, "vip", fmt.Sprintf("%t", *ip.Vip),
			),
			resource.TestCheckResourceAttr(
				res, "allocation_type", allocationType,
			),
			resource.TestCheckResourceAttr(
				res, "allocation_id", allocationID,
			),
		}

		if ip.Network != nil {
			tfs = append(tfs, resource.TestCheckResourceAttr(
				res, "network_id", *ip.Network.Id,
			))
		}

		return resource.ComposeAggregateTestCheckFunc(tfs...)(s)
	}
}

func testAccCheckKatapultIPDestroy(
	tt *testTools,
) resource.TestCheckFunc {
	m := tt.Meta

	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "katapult_ip" {
				continue
			}

			response, err := m.Core.GetIpAddressWithResponse(tt.Ctx,
				&core.GetIpAddressParams{
					IpAddressId: &rs.Primary.ID,
				})
			if err == nil && response.JSON200 != nil {
				return fmt.Errorf(
					"katapult_ip %s (%s) was not destroyed",
					rs.Primary.ID, *response.JSON200.IpAddress.Address)
			}
		}

		return nil
	}
}
