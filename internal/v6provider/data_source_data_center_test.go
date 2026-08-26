package v6provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jimeh/undent"
	core "github.com/krystal/go-katapult/next/core"
)

func TestAccKatapultDataSourceDataCenter_default(t *testing.T) {
	tt := newTestTools(t)
	dc := getKatapultDataCenter(tt, tt.Meta.confDataCenter)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "katapult_data_center" "main" {}`,
				Check: testAccCheckKatapultDataCenterAttrs(
					"data.katapult_data_center.main", dc,
				),
			},
		},
	})
}

func TestAccKatapultDataSourceDataCenter_by_id(t *testing.T) {
	tt := newTestTools(t)
	dc := getKatapultDataCenter(tt, tt.Meta.confDataCenter)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					data "katapult_data_center" "main" {
					  id = "%s"
					}`,
					*dc.Id,
				),
				Check: testAccCheckKatapultDataCenterAttrs(
					"data.katapult_data_center.main", dc,
				),
			},
		},
	})
}

func TestAccKatapultDataSourceDataCenter_by_permalink(t *testing.T) {
	tt := newTestTools(t)
	dc := getKatapultDataCenter(tt, tt.Meta.confDataCenter)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					data "katapult_data_center" "main" {
					  permalink = "%s"
					}`,
					*dc.Permalink,
				),
				Check: testAccCheckKatapultDataCenterAttrs(
					"data.katapult_data_center.main", dc,
				),
			},
		},
	})
}

func TestAccKatapultDataSourceDataCenter_invalid(t *testing.T) {
	tt := newTestTools(t).NoHTTP()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					data "katapult_data_center" "main" {
					  name = "London"
					}`),
				ExpectError: regexp.MustCompile(
					`(?i)read-only attribute`,
				),
			},
		},
	})
}

func testAccCheckKatapultDataCenterAttrs(
	resourceName string,
	dc *core.DataCenter,
) resource.TestCheckFunc {
	checks := []resource.TestCheckFunc{
		resource.TestCheckResourceAttr(resourceName, "id", *dc.Id),
		resource.TestCheckResourceAttr(resourceName, "name", *dc.Name),
		resource.TestCheckResourceAttr(resourceName, "permalink", *dc.Permalink),
	}
	if dc.Country != nil {
		checks = append(checks,
			resource.TestCheckResourceAttr(resourceName, "country_id", *dc.Country.Id),
			resource.TestCheckResourceAttr(resourceName, "country_name", *dc.Country.Name),
		)
	}

	return resource.ComposeAggregateTestCheckFunc(checks...)
}

func TestDataCenterDataSourceModelWithoutCountry(t *testing.T) {
	t.Parallel()

	model := dataCenterDataSourceModel(&core.GetDataCenter200ResponseDataCenter{
		Id:        ptr("loc_test"),
		Name:      ptr("Test"),
		Permalink: ptr("test-1"),
	})
	if !model.CountryID.IsNull() || !model.CountryName.IsNull() {
		t.Fatalf("expected null country fields, got %#v", model)
	}
}
