package v6provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceVirtualMachinePackage_selectors(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      `data "katapult_virtual_machine_package" "selected" {}`,
				ExpectError: regexp.MustCompile(`(?i)at least one.*id,permalink`),
			},
			{
				Config: undent.String(`
					data "katapult_virtual_machine_packages" "all" {}

					data "katapult_virtual_machine_package" "by_id" {
					  id = data.katapult_virtual_machine_packages.all.packages[0].id
					}

					data "katapult_virtual_machine_package" "by_permalink" {
					  permalink = data.katapult_virtual_machine_packages.all.packages[0].permalink
					}

					data "katapult_virtual_machine_package" "both" {
					  id        = data.katapult_virtual_machine_packages.all.packages[0].id
					  permalink = "ignored"
					}

					data "katapult_virtual_machine_package" "empty_id" {
					  id        = ""
					  permalink = data.katapult_virtual_machine_packages.all.packages[0].permalink
					}

					data "katapult_virtual_machine_package" "empty_permalink" {
					  id        = data.katapult_virtual_machine_packages.all.packages[0].id
					  permalink = ""
					}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.katapult_virtual_machine_packages.all", "id", "all",
					),
					resource.TestCheckResourceAttrSet(
						"data.katapult_virtual_machine_packages.all", "packages.0.id",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_virtual_machine_package.by_id", "id",
						"data.katapult_virtual_machine_packages.all", "packages.0.id",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_virtual_machine_package.by_id", "name",
						"data.katapult_virtual_machine_packages.all", "packages.0.name",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_virtual_machine_package.by_id", "permalink",
						"data.katapult_virtual_machine_packages.all", "packages.0.permalink",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_virtual_machine_package.by_id", "cpu_cores",
						"data.katapult_virtual_machine_packages.all", "packages.0.cpu_cores",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_virtual_machine_package.by_id", "ipv4_addresses",
						"data.katapult_virtual_machine_packages.all", "packages.0.ipv4_addresses",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_virtual_machine_package.by_id", "memory_in_gb",
						"data.katapult_virtual_machine_packages.all", "packages.0.memory_in_gb",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_virtual_machine_package.by_id", "storage_in_gb",
						"data.katapult_virtual_machine_packages.all", "packages.0.storage_in_gb",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_virtual_machine_package.by_id", "privacy",
						"data.katapult_virtual_machine_packages.all", "packages.0.privacy",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_virtual_machine_package.by_permalink", "id",
						"data.katapult_virtual_machine_packages.all", "packages.0.id",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_virtual_machine_package.both", "id",
						"data.katapult_virtual_machine_packages.all", "packages.0.id",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_virtual_machine_package.empty_id", "id",
						"data.katapult_virtual_machine_packages.all", "packages.0.id",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_virtual_machine_package.empty_permalink", "id",
						"data.katapult_virtual_machine_packages.all", "packages.0.id",
					),
				),
			},
		},
	})
}
