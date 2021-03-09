package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceVMGroup_basic(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName("data-source-basic")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckKatapultVMGroupDestroy(tt),
		),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_virtual_machine_group" "base" {
						name = "%s"
					}

					data "katapult_virtual_machine_group" "src" {
						id = katapult_virtual_machine_group.base.id
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVMGroupExists(
						tt, "data.katapult_virtual_machine_group.src",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_virtual_machine_group.src",
						"name", name,
					),
					resource.TestCheckResourceAttr(
						"data.katapult_virtual_machine_group.src",
						"segregate", "true",
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceVMGroup_not_segregated(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName("data-source-not-segregated")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckKatapultVMGroupDestroy(tt),
		),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_virtual_machine_group" "base" {
						name = "%s"
						segregate = false
					}

					data "katapult_virtual_machine_group" "src" {
						id = katapult_virtual_machine_group.base.id
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVMGroupExists(
						tt, "data.katapult_virtual_machine_group.src",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_virtual_machine_group.src",
						"name", name,
					),
					resource.TestCheckResourceAttr(
						"data.katapult_virtual_machine_group.src",
						"segregate", "false",
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceVMGroup_blank(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "katapult_virtual_machine_group" "src" {}`,
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						"The argument \"id\" is required, but no definition " +
							"was found.",
					),
				),
			},
		},
	})
}
