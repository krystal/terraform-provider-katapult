package v6provider

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccKatapultDataSourceDiskIOProfiles_all(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "katapult_disk_io_profiles" "all" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(
						"data.katapult_disk_io_profiles.all", "profiles.0.id",
					),
					resource.TestCheckResourceAttrSet(
						"data.katapult_disk_io_profiles.all", "profiles.0.name",
					),
					resource.TestCheckResourceAttrSet(
						"data.katapult_disk_io_profiles.all", "profiles.0.permalink",
					),
					testAccCheckDiskIOProfilesSorted(
						"data.katapult_disk_io_profiles.all",
					),
				),
			},
		},
	})
}

func testAccCheckDiskIOProfilesSorted(dataSourceAddress string) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		dataSource, ok := state.RootModule().Resources[dataSourceAddress]
		if !ok {
			return fmt.Errorf("resource not found: %s", dataSourceAddress)
		}

		count, _ := strconv.Atoi(dataSource.Primary.Attributes["profiles.#"])
		if count == 0 {
			return fmt.Errorf("no disk I/O profiles returned by %s", dataSourceAddress)
		}
		previous := ""
		for i := range count {
			id := dataSource.Primary.Attributes[fmt.Sprintf("profiles.%d.id", i)]
			if previous != "" && id < previous {
				return fmt.Errorf("profiles not sorted by ID: %q appears after %q", id, previous)
			}
			previous = id
		}
		return nil
	}
}
