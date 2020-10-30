package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/stretchr/testify/assert"
)

func TestAccKatapultDataSourceDataCenter_default(t *testing.T) {
	tt := NewTestTools(t)
	defer tt.Cleanup()

	res := "data.katapult_data_center.main"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "katapult_data_center" "main" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccKatapultCheckDataCenterExists(tt, res),
					resource.TestCheckResourceAttr(res,
						"id", testAccDataCenter["id"]),
					resource.TestCheckResourceAttr(res,
						"name", testAccDataCenter["name"]),
					resource.TestCheckResourceAttr(res,
						"permalink", testAccDataCenter["permalink"]),
					resource.TestCheckResourceAttr(res,
						"country_id", testAccDataCenter["country_id"]),
					resource.TestCheckResourceAttr(res,
						"country_name", testAccDataCenter["country_name"]),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceDataCenter_by_id(t *testing.T) {
	tt := NewTestTools(t)
	defer tt.Cleanup()

	res := "data.katapult_data_center.main"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: dedentf(`
					data "katapult_data_center" "main" {
					  id = "%s"
					}`,
					testAccDataCenter["id"],
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccKatapultCheckDataCenterExists(tt, res),
					resource.TestCheckResourceAttr(res,
						"id", testAccDataCenter["id"]),
					resource.TestCheckResourceAttr(res,
						"name", testAccDataCenter["name"]),
					resource.TestCheckResourceAttr(res,
						"permalink", testAccDataCenter["permalink"]),
					resource.TestCheckResourceAttr(res,
						"country_id", testAccDataCenter["country_id"]),
					resource.TestCheckResourceAttr(res,
						"country_name", testAccDataCenter["country_name"]),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceDataCenter_by_permalink(t *testing.T) {
	tt := NewTestTools(t)
	defer tt.Cleanup()

	res := "data.katapult_data_center.main"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: dedentf(`
					data "katapult_data_center" "main" {
					  permalink = "%s"
					}`,
					testAccDataCenter["permalink"],
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccKatapultCheckDataCenterExists(tt, res),
					resource.TestCheckResourceAttr(res,
						"id", testAccDataCenter["id"]),
					resource.TestCheckResourceAttr(res,
						"name", testAccDataCenter["name"]),
					resource.TestCheckResourceAttr(res,
						"permalink", testAccDataCenter["permalink"]),
					resource.TestCheckResourceAttr(res,
						"country_id", testAccDataCenter["country_id"]),
					resource.TestCheckResourceAttr(res,
						"country_name", testAccDataCenter["country_name"]),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceDataCenter_invalid(t *testing.T) {
	tt := NewTestTools(t)
	defer tt.Cleanup()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: dedentf(`
					data "katapult_data_center" "main" {
					  name = "%s"
					}`,
					testAccDataCenter["name"],
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta("Computed attribute cannot be set"),
				),
			},
		},
	})
}

func testAccKatapultCheckDataCenterExists(
	tt *TestTools,
	n string,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := tt.Meta.Client

		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("resource not found: %s", n)
		}

		lb, _, err := c.DataCenters.Get(tt.Meta.Ctx, rs.Primary.ID)
		if err != nil {
			return err
		}

		assert.Equal(tt.T, rs.Primary.Attributes["name"], lb.Name)
		assert.Equal(tt.T, rs.Primary.Attributes["permalink"], lb.Permalink)

		return nil
	}
}
