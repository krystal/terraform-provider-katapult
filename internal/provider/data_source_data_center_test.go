package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/core"
	"github.com/stretchr/testify/require"
)

func TestAccKatapultDataSourceDataCenter_default(t *testing.T) {
	tt := newTestTools(t)

	dc, _, err := tt.Meta.Core.DataCenters.Get(tt.Ctx, tt.Meta.DataCenterRef)
	require.NoError(t, err)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "katapult_data_center" "main" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultDataCenterExists(
						tt, "data.katapult_data_center.main",
					),
					testAccCheckKatapultDataCenterAttrs(
						"data.katapult_data_center.main", dc, "",
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceDataCenter_by_id(t *testing.T) {
	tt := newTestTools(t)

	dc, _, err := tt.Meta.Core.DataCenters.Get(tt.Ctx, tt.Meta.DataCenterRef)
	require.NoError(t, err)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					data "katapult_data_center" "main" {
					  id = "%s"
					}`,
					dc.ID,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultDataCenterExists(
						tt, "data.katapult_data_center.main",
					),
					testAccCheckKatapultDataCenterAttrs(
						"data.katapult_data_center.main", dc, "",
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceDataCenter_by_permalink(t *testing.T) {
	tt := newTestTools(t)

	dc, _, err := tt.Meta.Core.DataCenters.Get(tt.Ctx, tt.Meta.DataCenterRef)
	require.NoError(t, err)

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
					testAccCheckKatapultDataCenterExists(
						tt, "data.katapult_data_center.main",
					),
					testAccCheckKatapultDataCenterAttrs(
						"data.katapult_data_center.main", dc, "",
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceDataCenter_invalid(t *testing.T) {
	tt := newTestTools(t)

	dc, _, err := tt.Meta.Core.DataCenters.Get(tt.Ctx, tt.Meta.DataCenterRef)
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
					regexp.QuoteMeta(`Can't configure a value for "name"`),
				),
			},
		},
	})
}

func testAccCheckKatapultDataCenterExists(
	tt *testTools,
	res string,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := tt.Meta.Core

		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("resource not found: %s", res)
		}

		_, _, err := c.DataCenters.GetByID(tt.Ctx, rs.Primary.ID)

		return err
	}
}

func testAccCheckKatapultDataCenterAttrs(
	res string,
	dc *core.DataCenter,
	prefix string,
) resource.TestCheckFunc {
	tfs := []resource.TestCheckFunc{
		resource.TestCheckResourceAttr(res, prefix+"id", dc.ID),
		resource.TestCheckResourceAttr(res, prefix+"name", dc.Name),
		resource.TestCheckResourceAttr(res, prefix+"permalink", dc.Permalink),
	}

	if dc.Country != nil {
		tfs = append(tfs,
			resource.TestCheckResourceAttr(
				res, prefix+"country_id", dc.Country.ID,
			),
			resource.TestCheckResourceAttr(
				res, prefix+"country_name", dc.Country.Name,
			),
		)
	}

	return resource.ComposeAggregateTestCheckFunc(tfs...)
}
