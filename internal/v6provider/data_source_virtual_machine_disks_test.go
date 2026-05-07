package v6provider

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceVirtualMachineDisks_basic(t *testing.T) {
	tt := newTestTools(t)

	diskName := tt.ResourceName("disk")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckKatapultVirtualMachineDestroy(tt),
			testAccCheckKatapultDiskDestroy(tt),
			testAccCheckKatapultIPDestroy(tt),
		),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_ip" "web" {}

					resource "katapult_disk" "data" {
					  name       = "%s"
					  size_in_gb = 20
					}

					resource "katapult_virtual_machine" "base" {
					  package       = "rock-3"
					  disk_template = "ubuntu-18-04"
					  disk_template_options = {
					    install_agent = true
					  }
					  ip_address_ids = [katapult_ip.web.id]
					  disk_ids       = [katapult_disk.data.id]
					}

					data "katapult_virtual_machine_disks" "all" {
					  virtual_machine_id = katapult_virtual_machine.base.id
					}`, diskName,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.katapult_virtual_machine_disks.all",
						"disks.#", "2",
					),
					testAccCheckDataSourceDiskMatch(
						"data.katapult_virtual_machine_disks.all",
						"katapult_disk.data",
						false,
					),
					testAccCheckDataSourceHasOneBootDisk(
						"data.katapult_virtual_machine_disks.all",
					),
				),
			},
		},
	})
}

// testAccCheckDataSourceDiskMatch finds a disk in the data source matching the
// referenced standalone disk resource by ID, asserting boot equals expectBoot.
func testAccCheckDataSourceDiskMatch(
	dsAddr, diskAddr string,
	expectBoot bool,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ds, ok := s.RootModule().Resources[dsAddr]
		if !ok {
			return fmt.Errorf("not found: %s", dsAddr)
		}
		disk, ok := s.RootModule().Resources[diskAddr]
		if !ok {
			return fmt.Errorf("not found: %s", diskAddr)
		}
		want := disk.Primary.ID

		count, _ := strconv.Atoi(ds.Primary.Attributes["disks.#"])
		for i := 0; i < count; i++ {
			id := ds.Primary.Attributes[fmt.Sprintf("disks.%d.id", i)]
			if id != want {
				continue
			}
			boot := ds.Primary.Attributes[fmt.Sprintf("disks.%d.boot", i)] == "true"
			if boot != expectBoot {
				return fmt.Errorf(
					"disk %s: expected boot=%v, got boot=%v",
					id, expectBoot, boot,
				)
			}
			return nil
		}
		return fmt.Errorf(
			"disk %s not present in %s.disks",
			want, dsAddr,
		)
	}
}

func testAccCheckDataSourceHasOneBootDisk(
	dsAddr string,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ds, ok := s.RootModule().Resources[dsAddr]
		if !ok {
			return fmt.Errorf("not found: %s", dsAddr)
		}

		count, _ := strconv.Atoi(ds.Primary.Attributes["disks.#"])
		boots := 0
		for i := 0; i < count; i++ {
			if ds.Primary.Attributes[fmt.Sprintf("disks.%d.boot", i)] == "true" {
				boots++
			}
		}
		if boots != 1 {
			return fmt.Errorf(
				"expected exactly 1 boot disk, got %d", boots,
			)
		}
		return nil
	}
}
