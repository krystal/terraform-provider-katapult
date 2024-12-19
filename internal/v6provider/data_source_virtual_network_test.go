package v6provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceVirtualNetwork_by_id(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultVirtualNetworkDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_virtual_network" "main" {
					  name = "%s"
					}

					data "katapult_virtual_network" "main" {
					  id = katapult_virtual_network.main.id
					}
				`, name),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckKatapultVirtualNetworkAttrs(
						tt, "data.katapult_virtual_network.main", nil,
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_virtual_network.main", "id",
						"katapult_virtual_network.main", "id",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_virtual_network.main", "name",
						"katapult_virtual_network.main", "name",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_virtual_network.main", "data_center_id",
						"katapult_virtual_network.main", "data_center_id",
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceVirtualNetwork_not_found(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultVirtualNetworkDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					data "katapult_virtual_network" "main" {
					  id = "nosuchthing"
					}
				`),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta("resource not found"),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceVirtualNetwork_no_attributes(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultVirtualNetworkDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: `data "katapult_virtual_network" "main" {}`,
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta("The argument \"id\" is required"),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceVirtualNetwork_empty_id(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultVirtualNetworkDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					data "katapult_virtual_network" "main" {
					  id = ""
					}
				`),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta("Attribute id string cannot be empty"),
				),
			},
		},
	})
}
