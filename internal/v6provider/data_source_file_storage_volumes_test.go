package v6provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceFileStorageVolumes_default(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	first := name + "-1"
	second := name + "-2"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultFSVDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_file_storage_volume" "first" {
						name = "%s"
					}

					resource "katapult_file_storage_volume" "second" {
						name = "%s"

						# Ensure consistent ordering for testing purposes.
						depends_on = [katapult_file_storage_volume.first]
					}

					data "katapult_file_storage_volumes" "all" {
						# Ensure consistent ordering for testing purposes.
						depends_on = [katapult_file_storage_volume.second]
					}`,
					first, second,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckTypeSetElemNestedAttrs(
						"data.katapult_file_storage_volumes.all",
						"file_storage_volumes.*", map[string]string{
							"name": first,
						},
					),
					resource.TestCheckTypeSetElemNestedAttrs(
						"data.katapult_file_storage_volumes.all",
						"file_storage_volumes.*", map[string]string{
							"name": second,
						},
					),
				),
			},
		},
	})
}
