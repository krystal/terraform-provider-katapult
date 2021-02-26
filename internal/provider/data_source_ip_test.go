package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceIP_by_id(t *testing.T) {
	tt := NewTestTools(t)
	defer tt.Cleanup()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultIPDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					resource "katapult_ip" "main" {}

					data "katapult_ip" "src" {
					  id = katapult_ip.main.id
					}`,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultIPExists(tt, "data.katapult_ip.src"),
					resource.TestMatchResourceAttr(
						"data.katapult_ip.src", "address", regexp.MustCompile(
							`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$`,
						),
					),
					resource.TestCheckResourceAttrPair(
						"katapult_ip.main", "id",
						"data.katapult_ip.src", "id",
					),
					resource.TestCheckResourceAttrPair(
						"katapult_ip.main", "address",
						"data.katapult_ip.src", "address",
					),
					resource.TestCheckResourceAttrPair(
						"katapult_ip.main", "address_with_mask",
						"data.katapult_ip.src", "address_with_mask",
					),
					resource.TestCheckResourceAttrPair(
						"katapult_ip.main", "reverse_dns",
						"data.katapult_ip.src", "reverse_dns",
					),
					resource.TestCheckResourceAttrPair(
						"katapult_ip.main", "version",
						"data.katapult_ip.src", "version",
					),
					resource.TestCheckResourceAttrPair(
						"katapult_ip.main", "vip",
						"data.katapult_ip.src", "vip",
					),
					resource.TestCheckResourceAttrPair(
						"katapult_ip.main", "allocation_type",
						"data.katapult_ip.src", "allocation_type",
					),
					resource.TestCheckResourceAttrPair(
						"katapult_ip.main", "allocation_id",
						"data.katapult_ip.src", "allocation_id",
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceIP_by_address(t *testing.T) {
	tt := NewTestTools(t)
	defer tt.Cleanup()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultIPDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					resource "katapult_ip" "main" {}

					data "katapult_ip" "src" {
					  address = katapult_ip.main.address
					}`,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultIPExists(tt, "data.katapult_ip.src"),
					resource.TestMatchResourceAttr(
						"data.katapult_ip.src", "address", regexp.MustCompile(
							`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$`,
						),
					),
					resource.TestCheckResourceAttrPair(
						"katapult_ip.main", "id",
						"data.katapult_ip.src", "id",
					),
					resource.TestCheckResourceAttrPair(
						"katapult_ip.main", "address",
						"data.katapult_ip.src", "address",
					),
					resource.TestCheckResourceAttrPair(
						"katapult_ip.main", "address_with_mask",
						"data.katapult_ip.src", "address_with_mask",
					),
					resource.TestCheckResourceAttrPair(
						"katapult_ip.main", "reverse_dns",
						"data.katapult_ip.src", "reverse_dns",
					),
					resource.TestCheckResourceAttrPair(
						"katapult_ip.main", "version",
						"data.katapult_ip.src", "version",
					),
					resource.TestCheckResourceAttrPair(
						"katapult_ip.main", "vip",
						"data.katapult_ip.src", "vip",
					),
					resource.TestCheckResourceAttrPair(
						"katapult_ip.main", "allocation_type",
						"data.katapult_ip.src", "allocation_type",
					),
					resource.TestCheckResourceAttrPair(
						"katapult_ip.main", "allocation_id",
						"data.katapult_ip.src", "allocation_id",
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceIP_invalid(t *testing.T) {
	tt := NewTestTools(t)
	defer tt.Cleanup()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "katapult_ip" "src" {}`,
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta("one of `address,id` must be specified"),
				),
			},
		},
	})
}

//
// Helpers
//
