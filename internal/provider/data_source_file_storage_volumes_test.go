package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceFileStorageVolumes_default(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultFileStorageVolumeDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_legacy_file_storage_volume" "first" {
						name = "%s"
					}

					resource "katapult_legacy_file_storage_volume" "second" {
						name = "%s"
						# Ensure consistent ordering for testing purposes.
						depends_on = [katapult_legacy_file_storage_volume.first]
					}`,
					name+"-1", name+"-2",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultFileStorageVolumeAttrs(
						tt, "katapult_legacy_file_storage_volume.first",
					),
					testAccCheckKatapultFileStorageVolumeAttrs(
						tt, "katapult_legacy_file_storage_volume.second",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_legacy_file_storage_volume" "first" {
						name = "%s"
					}

					resource "katapult_legacy_file_storage_volume" "second" {
						name = "%s"

						# Ensure consistent ordering for testing purposes.
						depends_on = [katapult_legacy_file_storage_volume.first]
					}

					data "katapult_legacy_file_storage_volumes" "all" {}`,
					name+"-1", name+"-2",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.katapult_legacy_file_storage_volumes.all",
						"file_storage_volumes.0.name", name+"-1",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_legacy_file_storage_volumes.all",
						"file_storage_volumes.1.name", name+"-2",
					),
				),
			},
		},
	})
}
