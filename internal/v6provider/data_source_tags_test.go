package v6provider

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceTags_minimal(t *testing.T) {
	tt := newTestTools(t)

	name := strings.ToLower(tt.ResourceName())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultTagDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
			resource "katapult_tag" "database" {
				name = "%s"
				color = "green"
			}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultTagAttrs(tt, "katapult_tag.database"),
				),
			},
			{
				Config: undent.Stringf(`
			resource "katapult_tag" "database" {

				name = "%s"
				color = "green"
			}
			
			data "katapult_tags" "tags" {}
			`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.katapult_tags.tags", "tags.#", "6",
					),
				),
			},
		},
	})
}
