package v6provider

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceVMGroups_default(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultVMGroupDestroy(tt),
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
					testCheckVMGroupsListContains(
						"data.katapult_virtual_machine_groups.src",
						name+"-1", true,
					),
					testCheckVMGroupsListContains(
						"data.katapult_virtual_machine_groups.src",
						name+"-2", false,
					),
				),
			},
		},
	})
}

// testCheckVMGroupsListContains searches the groups list for an entry with the
// given name and segregate value, without assuming a specific list position.
func testCheckVMGroupsListContains(
	res, name string,
	segregate bool,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("resource not found: %s", res)
		}

		countStr := rs.Primary.Attributes["groups.#"]
		count, _ := strconv.Atoi(countStr)

		wantSegregate := "true"
		if !segregate {
			wantSegregate = "false"
		}

		for i := range count {
			if rs.Primary.Attributes[fmt.Sprintf("groups.%d.name", i)] == name {
				segregateKey := fmt.Sprintf("groups.%d.segregate", i)
				got := rs.Primary.Attributes[segregateKey]
				if got != wantSegregate {
					return fmt.Errorf(
						"group %q: segregate = %s, want %s",
						name, got, wantSegregate,
					)
				}
				return nil
			}
		}

		return fmt.Errorf("no group with name %q found in %s", name, res)
	}
}
