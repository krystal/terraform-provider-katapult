package v6provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceDiskTemplate_migrate_v5_state(t *testing.T) {
	config := undent.String(`
		data "katapult_disk_templates" "all" {}

		data "katapult_disk_templates" "organization" {
		  include_universal = false
		}

		data "katapult_disk_template" "by_id" {
		  id = data.katapult_disk_templates.all.templates[0].id
		}

		data "katapult_disk_template" "by_permalink" {
		  permalink = data.katapult_disk_templates.all.templates[0].permalink
		}

		data "katapult_disk_template" "empty_id" {
		  id        = ""
		  permalink = data.katapult_disk_templates.all.templates[0].permalink
		}`)

	commonCheck := func(tt *testTools) resource.TestCheckFunc {
		return resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr(
				"data.katapult_disk_templates.all", "id", tt.Meta.confOrganization,
			),
			resource.TestCheckResourceAttr(
				"data.katapult_disk_templates.all", "include_universal", "true",
			),
			resource.TestCheckResourceAttr(
				"data.katapult_disk_templates.organization", "include_universal", "false",
			),
			resource.TestCheckResourceAttrSet(
				"data.katapult_disk_templates.all", "templates.0.id",
			),
			resource.TestCheckResourceAttrPair(
				"data.katapult_disk_template.by_id", "id",
				"data.katapult_disk_templates.all", "templates.0.id",
			),
			resource.TestCheckResourceAttrPair(
				"data.katapult_disk_template.by_id", "description",
				"data.katapult_disk_templates.all", "templates.0.description",
			),
			resource.TestCheckResourceAttrPair(
				"data.katapult_disk_template.by_permalink", "id",
				"data.katapult_disk_templates.all", "templates.0.id",
			),
			resource.TestCheckResourceAttrPair(
				"data.katapult_disk_template.empty_id", "id",
				"data.katapult_disk_templates.all", "templates.0.id",
			),
		)
	}
	legacyCheck := func(tt *testTools) resource.TestCheckFunc {
		return resource.ComposeAggregateTestCheckFunc(
			commonCheck(tt),
			resource.TestCheckResourceAttr(
				"data.katapult_disk_template.by_id", "template_version", "0",
			),
		)
	}
	frameworkCheck := func(tt *testTools) resource.TestCheckFunc {
		return resource.ComposeAggregateTestCheckFunc(
			commonCheck(tt),
			resource.TestCheckResourceAttrPair(
				"data.katapult_disk_template.by_id", "template_version",
				"data.katapult_disk_templates.all", "templates.0.template_version",
			),
		)
	}
	runDataSourceV5Handover(t, config, legacyCheck, frameworkCheck)
}
