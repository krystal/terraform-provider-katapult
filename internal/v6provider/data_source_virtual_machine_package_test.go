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
						"data.katapult_virtual_machine_package.by_permalink", "id",
						"data.katapult_virtual_machine_packages.all", "packages.0.id",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_virtual_machine_package.both", "id",
						"data.katapult_virtual_machine_packages.all", "packages.0.id",
					),
				),
			},
		},
	})
}
