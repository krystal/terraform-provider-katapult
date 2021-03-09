package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceVMGroups_default(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName("data-source-group")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_virtual_machine_group" "first" {
						name = "%s"
					}

					resource "katapult_virtual_machine_group" "second" {
						name = "%s"
						segregate = false

						depends_on = [
							katapult_virtual_machine_group.first,
						]
					}`,
					name+"-1", name+"-2",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVMGroupExists(
						tt, "katapult_virtual_machine_group.first",
					),
					testAccCheckKatapultVMGroupExists(
						tt, "katapult_virtual_machine_group.second",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_virtual_machine_group" "first" {
						name = "%s"
					}

					resource "katapult_virtual_machine_group" "second" {
						name = "%s"
						segregate = false

						depends_on = [
							katapult_virtual_machine_group.first,
						]
					}

					data "katapult_virtual_machine_groups" "src" {}`,
					name+"-1", name+"-2",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.katapult_virtual_machine_groups.src",
						"id", tt.Meta.confOrganization,
					),
					resource.TestCheckResourceAttr(
						"data.katapult_virtual_machine_groups.src",
						"groups.0.name", name+"-1",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_virtual_machine_groups.src",
						"groups.0.segregate", "true",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_virtual_machine_groups.src",
						"groups.1.name", name+"-2",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_virtual_machine_groups.src",
						"groups.1.segregate", "false",
					),
				),
			},
		},
	})
}
