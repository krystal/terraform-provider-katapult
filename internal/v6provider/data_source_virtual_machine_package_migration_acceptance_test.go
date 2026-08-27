package v6provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceVirtualMachinePackage_migrate_v5_state(t *testing.T) {
	config := undent.String(`
		data "katapult_virtual_machine_packages" "all" {}

		data "katapult_virtual_machine_package" "by_id" {
		  id = data.katapult_virtual_machine_packages.all.packages[0].id
		}

		data "katapult_virtual_machine_package" "by_permalink" {
		  permalink = data.katapult_virtual_machine_packages.all.packages[0].permalink
		}

		data "katapult_virtual_machine_package" "empty_permalink" {
		  id        = data.katapult_virtual_machine_packages.all.packages[0].id
		  permalink = ""
		}`)

	check := func(_ *testTools) resource.TestCheckFunc {
		return resource.ComposeAggregateTestCheckFunc(
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
				"data.katapult_virtual_machine_package.by_id", "cpu_cores",
				"data.katapult_virtual_machine_packages.all", "packages.0.cpu_cores",
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
				"data.katapult_virtual_machine_package.by_permalink", "id",
				"data.katapult_virtual_machine_packages.all", "packages.0.id",
			),
			resource.TestCheckResourceAttrPair(
				"data.katapult_virtual_machine_package.empty_permalink", "id",
				"data.katapult_virtual_machine_packages.all", "packages.0.id",
			),
		)
	}
	runDataSourceV5Handover(t, config, check, check)
}
