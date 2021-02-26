package provider

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/pkg/katapult"
	"github.com/stretchr/testify/require"
)

func init() { //nolint:gochecknoinits
	resource.AddTestSweepers("katapult_ip", &resource.Sweeper{
		Name: "katapult_ip",
		F:    testSweepIPs,
	})
}

func testSweepIPs(_ string) error {
	m := sweepMeta()
	ctx := context.TODO()

	var ips []*katapult.IPAddress
	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		pageResult, resp, err := m.Client.IPAddresses.List(
			ctx, m.OrganizationRef(),
			&katapult.ListOptions{Page: pageNum},
		)
		if err != nil {
			return err
		}

		totalPages = resp.Pagination.TotalPages
		ips = append(ips, pageResult...)
	}

	for _, ip := range ips {
		log.Printf(
			"[DEBUG]  - Deleting IP Address %s (%s)\n", ip.ID, ip.Address,
		)
		_, err := m.Client.IPAddresses.Delete(ctx, ip)
		if err != nil {
			return err
		}
	}

	return nil
}

func TestAccKatapultIP_basic(t *testing.T) {
	tt := newTestTools(t)

	network, err := defaultNetworkForDataCenter(tt.Ctx, tt.Meta)
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
					testAccCheckKatapultIPExists(tt, "katapult_ip.vip"),
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

func TestAccKatapultIP_with_network_id(t *testing.T) {
	tt := newTestTools(t)

	network, err := defaultNetworkForDataCenter(tt.Ctx, tt.Meta)
	require.NoError(t, err)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultIPDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_ip" "with-net" {
					  network_id = "%s"
					}`,
					network.ID,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultIPExists(tt, "katapult_ip.with-net"),
					resource.TestCheckResourceAttr(
						"katapult_ip.with-net", "network_id", network.ID,
					),
				),
			},
			{
				ResourceName:      "katapult_ip.with-net",
				ImportState:       true,
				ImportStateVerify: true,
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

		ip, _, err := m.Client.IPAddresses.GetByID(tt.Ctx, rs.Primary.ID)
		if err != nil {
			return err
		}

		return resource.TestCheckResourceAttr(res, "id", ip.ID)(s)
	}
}

func testAccCheckKatapultIPAttrs(
	tt *testTools,
	res string,
	ip *katapult.IPAddress,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if ip == nil {
			rs, ok := s.RootModule().Resources[res]
			if !ok {
				return fmt.Errorf("resource not found: %s", res)
			}

			var err error
			ip, _, err = tt.Meta.Client.IPAddresses.GetByID(
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

			ip, _, err := m.Client.IPAddresses.GetByID(tt.Ctx, rs.Primary.ID)
			if err == nil && ip != nil {
				return fmt.Errorf(
					"katapult_ip %s (%s) was not destroyed",
					rs.Primary.ID, ip.Address)
			}
		}

		return nil
	}
}
