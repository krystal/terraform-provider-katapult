package v6provider

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceTag_minimal(t *testing.T) {
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
			}

			data "katapult_tag" "db" {
				id = katapult_tag.database.id
			}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultTagAttrs(
						tt, "data.katapult_tag.db",
					),
				),
			},
		},
	})
}
