package provider

import (
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

	var ips []*katapult.IPAddress
	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		pageResult, resp, err := m.Client.IPAddresses.List(
			m.Ctx,
			&katapult.Organization{ID: m.OrganizationID},
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
		_, err := m.Client.IPAddresses.Delete(m.Ctx, ip)
		if err != nil {
			return err
		}
	}

	return nil
}

func TestAccKatapultIP_basic(t *testing.T) {
	tt := NewTestTools(t)
	defer tt.Cleanup()

	res := "katapult_ip.web"

	network, err := defaultNetworkForDataCenter(
		tt.Meta.Ctx, tt.Meta, tt.Meta.DataCenter(),
	)
	require.NoError(t, err)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `resource "katapult_ip" "web" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccKatapultCheckIPAddressExists(tt, res),
					resource.TestCheckResourceAttr(
						res, "network_id", network.ID,
					),
				),
			},
			{
				ResourceName:      res,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultIP_vip(t *testing.T) {
	tt := NewTestTools(t)
	defer tt.Cleanup()

	name := tt.ResourceName("web-vip")
	res := "katapult_ip.vip"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
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
					testAccKatapultCheckIPAddressExists(tt, res),
					resource.TestCheckResourceAttr(res, "vip", "true"),
					resource.TestCheckResourceAttr(res, "label", name),
				),
			},
			{
				ResourceName:      res,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultIP_with_network_id(t *testing.T) {
	tt := NewTestTools(t)
	defer tt.Cleanup()

	res := "katapult_ip.with-net"

	network, err := defaultNetworkForDataCenter(
		tt.Meta.Ctx, tt.Meta, tt.Meta.DataCenter(),
	)
	require.NoError(t, err)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_ip" "with-net" {
					  network_id = "%s"
					}`,
					network.ID,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccKatapultCheckIPAddressExists(tt, res),
					resource.TestCheckResourceAttr(
						res, "network_id", network.ID,
					),
				),
			},
			{
				ResourceName:      res,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

//
// Helpers
//

func testAccKatapultCheckIPAddressExists(
	tt *TestTools,
	res string,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := tt.Meta.Client

		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("resource not found: %s", res)
		}

		obj, _, err := c.IPAddresses.GetByID(tt.Meta.Ctx, rs.Primary.ID)
		if err != nil {
			return err
		}

		if obj.Network != nil {
			if rs.Primary.Attributes["network_id"] != obj.Network.ID {
				return fmt.Errorf(
					"expected network_id to be \"%s\", got \"%s\"",
					obj.Network.ID, rs.Primary.Attributes["network_id"],
				)
			}
		}

		if rs.Primary.Attributes["address"] != obj.Address {
			return fmt.Errorf(
				"expected address to be \"%s\", got \"%s\"",
				obj.Address, rs.Primary.Attributes["address"],
			)
		}

		if rs.Primary.Attributes["address_with_mask"] != obj.AddressWithMask {
			return fmt.Errorf(
				"expected address_with_mask to be \"%s\", got \"%s\"",
				obj.AddressWithMask, rs.Primary.Attributes["address_with_mask"],
			)
		}

		if rs.Primary.Attributes["reverse_dns"] != obj.ReverseDNS {
			return fmt.Errorf(
				"expected reverse_dns to be \"%s\", got \"%s\"",
				obj.ReverseDNS, rs.Primary.Attributes["reverse_dns"],
			)
		}

		if rs.Primary.Attributes["version"] != strconv.Itoa(
			flattenIPVersion(obj.Address),
		) {
			return fmt.Errorf(
				"expected version to be \"%s\", got \"%s\"",
				strconv.Itoa(flattenIPVersion(obj.Address)),
				rs.Primary.Attributes["version"],
			)
		}

		if rs.Primary.Attributes["vip"] != fmt.Sprintf("%t", obj.VIP) {
			return fmt.Errorf(
				"expected vip to be \"%s\", got \"%s\"",
				fmt.Sprintf("%t", obj.VIP),
				rs.Primary.Attributes["vip"],
			)
		}

		if rs.Primary.Attributes["allocation_type"] != obj.AllocationType {
			return fmt.Errorf(
				"expected allocation_type to be \"%s\", got \"%s\"",
				obj.AllocationType, rs.Primary.Attributes["allocation_type"],
			)
		}

		if rs.Primary.Attributes["allocation_id"] != obj.AllocationID {
			return fmt.Errorf(
				"expected allocation_id to be \"%s\", got \"%s\"",
				obj.AllocationID, rs.Primary.Attributes["allocation_id"],
			)
		}

		return nil
	}
}
