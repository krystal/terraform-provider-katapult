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

	res := "data.katapult_ip.src"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					resource "katapult_ip" "main" {}

					data "katapult_ip" "src" {
					  id = katapult_ip.main.id
					}`,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccKatapultCheckIPAddressExists(tt, res),
					resource.TestMatchResourceAttr(
						res, "address", regexp.MustCompile(
							`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$`,
						),
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceIP_by_address(t *testing.T) {
	tt := NewTestTools(t)
	defer tt.Cleanup()

	res := "data.katapult_ip.src"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					resource "katapult_ip" "main" {}

					data "katapult_ip" "src" {
					  address = katapult_ip.main.address
					}`,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccKatapultCheckIPAddressExists(tt, res),
					resource.TestMatchResourceAttr(
						res, "address", regexp.MustCompile(
							`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$`,
						),
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
