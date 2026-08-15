package v6provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceDisk_basic(t *testing.T) {
	tt := newTestTools(t)
	name := tt.ResourceName("data-source")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultDiskDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config:      `data "katapult_disk" "missing" { id = "disk_missing" }`,
				ExpectError: regexp.MustCompile(`(?i)(disk_missing|not found)`),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_disk" "data" {
					  name       = "%s"
					  size_in_gb = 20
					}

					data "katapult_disk" "data" {
					  id = katapult_disk.data.id
					}`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						"data.katapult_disk.data", "id", "katapult_disk.data", "id",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_disk.data", "name", "katapult_disk.data", "name",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_disk.data", "size_in_gb", "katapult_disk.data", "size_in_gb",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_disk.data", "storage_speed", "katapult_disk.data", "storage_speed",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_disk.data", "bus_type", "katapult_disk.data", "bus_type",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_disk.data", "io_profile_id", "katapult_disk.data", "io_profile_id",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_disk.data", "wwn", "katapult_disk.data", "wwn",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_disk.data", "state", "katapult_disk.data", "state",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_disk.data", "data_center_permalink", tt.Meta.confDataCenter,
					),
					resource.TestCheckNoResourceAttr("data.katapult_disk.data", "virtual_machine_id"),
					resource.TestCheckNoResourceAttr("data.katapult_disk.data", "attachment_state"),
				),
			},
		},
	})
}
