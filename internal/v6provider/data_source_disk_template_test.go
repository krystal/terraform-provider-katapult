package v6provider

import (
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceDiskTemplate_selectorsAndFilters(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      `data "katapult_disk_template" "selected" {}`,
				ExpectError: regexp.MustCompile(`(?i)at least one.*id,permalink`),
			},
			{
				Config: undent.String(`
					data "katapult_disk_templates" "all" {}

					data "katapult_disk_template" "by_id" {
					  id = data.katapult_disk_templates.all.templates[0].id
					}

					data "katapult_disk_template" "by_permalink" {
					  permalink = data.katapult_disk_templates.all.templates[0].permalink
					}

					data "katapult_disk_template" "both" {
					  id        = data.katapult_disk_templates.all.templates[0].id
					  permalink = "ignored"
					}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.katapult_disk_templates.all",
						"id", tt.Meta.confOrganization,
					),
					resource.TestCheckResourceAttr(
						"data.katapult_disk_templates.all", "include_universal", "true",
					),
					resource.TestCheckResourceAttrSet(
						"data.katapult_disk_templates.all", "templates.0.id",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_disk_template.by_id", "id",
						"data.katapult_disk_templates.all", "templates.0.id",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_disk_template.by_permalink", "id",
						"data.katapult_disk_templates.all", "templates.0.id",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_disk_template.both", "id",
						"data.katapult_disk_templates.all", "templates.0.id",
					),
				),
			},
			{
				Config: undent.String(`
					data "katapult_disk_templates" "organization" {
					  include_universal = false
					}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.katapult_disk_templates.organization",
						"include_universal", "false",
					),
					testAccCheckNoUniversalDiskTemplates(
						"data.katapult_disk_templates.organization",
					),
				),
			},
		},
	})
}

func testAccCheckNoUniversalDiskTemplates(
	dataSourceAddress string,
) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		dataSource, ok := state.RootModule().Resources[dataSourceAddress]
		if !ok {
			return fmt.Errorf("resource not found: %s", dataSourceAddress)
		}

		count, _ := strconv.Atoi(dataSource.Primary.Attributes["templates.#"])
		for i := range count {
			key := fmt.Sprintf("templates.%d.universal", i)
			if dataSource.Primary.Attributes[key] == "true" {
				return fmt.Errorf("universal disk template returned at %s", key)
			}
		}
		return nil
	}
}
