package v6provider

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceDisks_all(t *testing.T) {
	tt := newTestTools(t)
	name := tt.ResourceName("collection")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultDiskDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_disk" "data" {
					  name       = "%s"
					  size_in_gb = 20
					}

					data "katapult_disks" "all" {
					  depends_on = [katapult_disk.data]
					}`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckDiskCollectionContains(
						"data.katapult_disks.all", "katapult_disk.data",
					),
					testAccCheckDiskCollectionSorted("data.katapult_disks.all"),
				),
			},
		},
	})
}

func testAccCheckDiskCollectionContains(
	dataSourceAddress string,
	diskAddress string,
) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		dataSource, ok := state.RootModule().Resources[dataSourceAddress]
		if !ok {
			return fmt.Errorf("resource not found: %s", dataSourceAddress)
		}
		disk, ok := state.RootModule().Resources[diskAddress]
		if !ok {
			return fmt.Errorf("resource not found: %s", diskAddress)
		}

		count, _ := strconv.Atoi(dataSource.Primary.Attributes["disks.#"])
		for i := range count {
			prefix := fmt.Sprintf("disks.%d.", i)
			if dataSource.Primary.Attributes[prefix+"id"] != disk.Primary.ID {
				continue
			}
			if got, want := dataSource.Primary.Attributes[prefix+"name"],
				disk.Primary.Attributes["name"]; got != want {
				return fmt.Errorf("disk %s name = %q, want %q", disk.Primary.ID, got, want)
			}
			return nil
		}

		return fmt.Errorf("disk %s not found in %s", disk.Primary.ID, dataSourceAddress)
	}
}

func testAccCheckDiskCollectionSorted(dataSourceAddress string) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		dataSource, ok := state.RootModule().Resources[dataSourceAddress]
		if !ok {
			return fmt.Errorf("resource not found: %s", dataSourceAddress)
		}

		count, _ := strconv.Atoi(dataSource.Primary.Attributes["disks.#"])
		previous := ""
		for i := range count {
			id := dataSource.Primary.Attributes[fmt.Sprintf("disks.%d.id", i)]
			if previous != "" && id < previous {
				return fmt.Errorf("disks not sorted by ID: %q appears after %q", id, previous)
			}
			previous = id
		}
		return nil
	}
}
