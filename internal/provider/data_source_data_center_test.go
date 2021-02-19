package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/jimeh/undent"
	"github.com/stretchr/testify/require"
)

func TestAccKatapultDataSourceDataCenter_default(t *testing.T) {
	tt := NewTestTools(t)
	defer tt.Cleanup()

	dcID, err := tt.Meta.DataCenterID(tt.Meta.Ctx)
	require.NoError(t, err)

	res := "data.katapult_data_center.main"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "katapult_data_center" "main" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccKatapultCheckDataCenterExists(tt, res, ""),
					resource.TestCheckResourceAttr(res, "id", dcID),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceDataCenter_by_id(t *testing.T) {
	tt := NewTestTools(t)
	defer tt.Cleanup()

	dcID, err := tt.Meta.DataCenterID(tt.Meta.Ctx)
	require.NoError(t, err)

	res := "data.katapult_data_center.main"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					data "katapult_data_center" "main" {
					  id = "%s"
					}`,
					dcID,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccKatapultCheckDataCenterExists(tt, res, dcID),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceDataCenter_by_permalink(t *testing.T) {
	tt := NewTestTools(t)
	defer tt.Cleanup()

	dc, err := tt.Meta.DataCenter(tt.Meta.Ctx)
	require.NoError(t, err)

	res := "data.katapult_data_center.main"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					data "katapult_data_center" "main" {
					  permalink = "%s"
					}`,
					dc.Permalink,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccKatapultCheckDataCenterExists(tt, res, dc.ID),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceDataCenter_invalid(t *testing.T) {
	tt := NewTestTools(t)
	defer tt.Cleanup()

	dc, err := tt.Meta.DataCenter(tt.Meta.Ctx)
	require.NoError(t, err)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					data "katapult_data_center" "main" {
					  name = "%s"
					}`,
					dc.Name,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta("Computed attributes cannot be set"),
				),
			},
		},
	})
}

func testAccKatapultCheckDataCenterExists(
	tt *TestTools,
	res string,
	id string,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := tt.Meta.Client

		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("resource not found: %s", res)
		}

		if id == "" {
			id = rs.Primary.ID
		}

		obj, _, err := c.DataCenters.GetByID(tt.Meta.Ctx, id)
		if err != nil {
			return err
		}

		if rs.Primary.Attributes["id"] != obj.ID {
			return fmt.Errorf(
				"expected id to be \"%s\", got \"%s\"",
				obj.ID, rs.Primary.Attributes["id"],
			)
		}

		if rs.Primary.Attributes["name"] != obj.Name {
			return fmt.Errorf(
				"expected name to be \"%s\", got \"%s\"",
				obj.Name, rs.Primary.Attributes["name"],
			)
		}

		if rs.Primary.Attributes["permalink"] != obj.Permalink {
			return fmt.Errorf(
				"expected permalink attribute to be \"%s\", got \"%s\"",
				obj.Permalink, rs.Primary.Attributes["permalink"],
			)
		}

		if obj.Country != nil {
			if rs.Primary.Attributes["country_id"] != obj.Country.ID {
				return fmt.Errorf(
					"expected country_id attribute to be \"%s\", got \"%s\"",
					obj.Country.ID, rs.Primary.Attributes["country_id"],
				)
			}

			if rs.Primary.Attributes["country_name"] != obj.Country.Name {
				return fmt.Errorf(
					"expected country_name attribute to be \"%s\", got \"%s\"",
					obj.Country.Name, rs.Primary.Attributes["country_name"],
				)
			}
		}

		return nil
	}
}
