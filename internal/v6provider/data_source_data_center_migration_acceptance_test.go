package v6provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceDataCenter_migrate_v5_state(t *testing.T) {
	config := undent.String(`
		data "katapult_data_center" "default" {}

		data "katapult_data_center" "by_id" {
		  id = data.katapult_data_center.default.id
		}

		data "katapult_data_center" "by_permalink" {
		  permalink = data.katapult_data_center.default.permalink
		}

		data "katapult_data_center" "empty_selectors" {
		  id        = ""
		  permalink = ""
		}`)

	check := func(_ *testTools) resource.TestCheckFunc {
		return resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttrSet("data.katapult_data_center.default", "id"),
			resource.TestCheckResourceAttrPair(
				"data.katapult_data_center.by_id", "id",
				"data.katapult_data_center.default", "id",
			),
			resource.TestCheckResourceAttrPair(
				"data.katapult_data_center.by_permalink", "id",
				"data.katapult_data_center.default", "id",
			),
			resource.TestCheckResourceAttrPair(
				"data.katapult_data_center.empty_selectors", "id",
				"data.katapult_data_center.default", "id",
			),
		)
	}
	runDataSourceV5Handover(t, config, check, check)
}
