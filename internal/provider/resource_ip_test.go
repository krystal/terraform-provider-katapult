package provider

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/core"
	"github.com/stretchr/testify/require"
)

func init() { //nolint:gochecknoinits
	resource.AddTestSweepers("katapult_ip", &resource.Sweeper{
		Name: "katapult_ip",
		F:    testSweepIPs,
		Dependencies: []string{
			"katapult_virtual_machine",
			"katapult_load_balancer",
		},
	})
}

func testSweepIPs(_ string) error {
	m := sweepMeta()
	ctx := context.TODO()

	var ips []*core.IPAddress
	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		pageResult, resp, err := m.Core.IPAddresses.List(
			ctx, m.OrganizationRef,
			&core.ListOptions{Page: pageNum},
		)
		if err != nil {
			return err
		}

		totalPages = resp.Pagination.TotalPages
		ips = append(ips, pageResult...)
	}

	for _, ip := range ips {
		if ip.AllocationID != "" {
			log.Printf(
				"[DEBUG]  - Skipping IP Address %s (%s), "+
					"allocated to %s (%s)\n",
				ip.ID, ip.Address, ip.AllocationID, ip.AllocationType,
			)

			continue
		}

		log.Printf(
			"[DEBUG]  - Deleting IP Address %s (%s)\n", ip.ID, ip.Address,
		)
		_, err := m.Core.IPAddresses.Delete(ctx, ip.Ref())
		if err != nil {
			return err
		}
	}

	return nil
}

func TestAccKatapultIP_minimal(t *testing.T) {
	tt := newTestTools(t)

	network, _, err := tt.Meta.Core.DataCenters.DefaultNetwork(
		tt.Ctx, tt.Meta.DataCenterRef,
	)
	require.NoError(t, err)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultIPDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: `resource "katapult_ip" "web" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultIPAttrs(tt, "katapult_ip.web", nil),
					resource.TestCheckResourceAttr(
						"katapult_ip.web", "network_id", network.ID,
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

	network, _, err := tt.Meta.Core.DataCenters.DefaultNetwork(
		tt.Ctx, tt.Meta.DataCenterRef,
	)
	require.NoError(t, err)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultIPDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					resource "katapult_ip" "web" {
						version = 4
					}`,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultIPAttrs(tt, "katapult_ip.web", nil),
					resource.TestCheckResourceAttr(
						"katapult_ip.web", "network_id", network.ID,
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

	network, _, err := tt.Meta.Core.DataCenters.DefaultNetwork(
		tt.Ctx, tt.Meta.DataCenterRef,
	)
	require.NoError(t, err)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultIPDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					resource "katapult_ip" "web" {
						version = 6
					}`,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultIPAttrs(tt, "katapult_ip.web", nil),
					resource.TestCheckResourceAttr(
						"katapult_ip.web", "network_id", network.ID,
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
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultIPDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					resource "katapult_ip" "web" {
						version = 5
					}`,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						"expected version to be one of [4 6], got 5",
					),
				),
			},
		},
	})
}

func TestAccKatapultIP_vip(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName("web-vip")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultIPDestroy(tt),
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
					testAccCheckKatapultIPAttrs(tt, "katapult_ip.vip", nil),
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
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultIPDestroy(tt),
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
						`expected "label" to not be an empty string, got`,
					),
				),
			},
		},
	})
}

func TestAccKatapultIP_vip_without_label(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultIPDestroy(tt),
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
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultIPDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					resource "katapult_ip" "vip" {
					  label = "hello"
					}`,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta("all of `label,vip` must be specified"),
				),
			},
		},
	})
}

func TestAccKatapultIP_with_network_id(t *testing.T) {
	tt := newTestTools(t)

	network, _, err := tt.Meta.Core.DataCenters.DefaultNetwork(
		tt.Ctx, tt.Meta.DataCenterRef,
	)
	require.NoError(t, err)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultIPDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_ip" "net" {
					  network_id = "%s"
					}`,
					network.ID,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultIPAttrs(tt, "katapult_ip.net", nil),
					resource.TestCheckResourceAttr(
						"katapult_ip.net", "network_id", network.ID,
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
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultIPDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: `resource "katapult_ip" "update" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultIPAttrs(tt, "katapult_ip.update", nil),
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
					testAccCheckKatapultIPAttrs(tt, "katapult_ip.update", nil),
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
					testAccCheckKatapultIPAttrs(tt, "katapult_ip.update", nil),
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
					testAccCheckKatapultIPAttrs(tt, "katapult_ip.update", nil),
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

		ip, _, err := m.Core.IPAddresses.GetByID(tt.Ctx, rs.Primary.ID)
		if err != nil {
			return err
		}

		return resource.TestCheckResourceAttr(res, "id", ip.ID)(s)
	}
}

func testAccCheckKatapultIPAttrs(
	tt *testTools,
	res string,
	ip *core.IPAddress,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if ip == nil {
			rs, ok := s.RootModule().Resources[res]
			if !ok {
				return fmt.Errorf("resource not found: %s", res)
			}

			var err error
			ip, _, err = tt.Meta.Core.IPAddresses.GetByID(
				tt.Ctx, rs.Primary.ID,
			)
			if err != nil {
				return err
			}
		}

		tfs := []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(res, "id", ip.ID),
			resource.TestCheckResourceAttr(res, "address", ip.Address),
			resource.TestCheckResourceAttr(
				res, "address_with_mask", ip.AddressWithMask,
			),
			resource.TestCheckResourceAttr(res, "reverse_dns", ip.ReverseDNS),
			resource.TestCheckResourceAttr(
				res, "version", strconv.Itoa(flattenIPVersion(ip.Address)),
			),
			resource.TestCheckResourceAttr(
				res, "vip", fmt.Sprintf("%t", ip.VIP),
			),
			resource.TestCheckResourceAttr(
				res, "allocation_type", ip.AllocationType,
			),
			resource.TestCheckResourceAttr(
				res, "allocation_id", ip.AllocationID,
			),
		}

		if ip.Network != nil {
			tfs = append(tfs, resource.TestCheckResourceAttr(
				res, "network_id", ip.Network.ID,
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

			ip, _, err := m.Core.IPAddresses.GetByID(tt.Ctx, rs.Primary.ID)
			if err == nil && ip != nil {
				return fmt.Errorf(
					"katapult_ip %s (%s) was not destroyed",
					rs.Primary.ID, ip.Address)
			}
		}

		return nil
	}
}
