package provider

import (
	"errors"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/core"
	"github.com/stretchr/testify/require"
)

func TestAccKatapultDataSourceDiskTemplates_default(t *testing.T) {
	tt := newTestTools(t)

	tpls, err := testHelperFetchAllDiskTemplates(tt)
	require.NoError(t, err)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "katapult_disk_templates" "main" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultDiskTemplates(
						"data.katapult_disk_templates.main", tpls,
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceDiskTemplates_include_universal(t *testing.T) {
	tt := newTestTools(t)

	tpls, err := testHelperFetchAllDiskTemplates(tt)
	require.NoError(t, err)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					data "katapult_disk_templates" "main" {
						include_universal = true
					}`,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultDiskTemplates(
						"data.katapult_disk_templates.main", tpls,
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceDiskTemplates_exclude_universal(t *testing.T) {
	tt := newTestTools(t)

	allTpls, err := testHelperFetchAllDiskTemplates(tt)
	require.NoError(t, err)

	tpls := []*core.DiskTemplate{}
	for _, t := range allTpls {
		if !t.Universal {
			tpls = append(tpls, t)
		}
	}

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					data "katapult_disk_templates" "main" {
						include_universal = false
					}`,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultDiskTemplates(
						"data.katapult_disk_templates.main", tpls,
					),
				),
			},
		},
	})
}

//
// Helpers
//

func testHelperFetchAllDiskTemplates(
	tt *testTools,
) ([]*core.DiskTemplate, error) {
	var templates []*core.DiskTemplate
	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		pageResult, resp, err := tt.Meta.Core.DiskTemplates.List(
			tt.Ctx, tt.Meta.OrganizationRef,
			&core.DiskTemplateListOptions{
				IncludeUniversal: true,
				Page:             pageNum,
			},
		)
		if err != nil {
			return nil, err
		}

		totalPages = resp.Pagination.TotalPages
		templates = append(templates, pageResult...)
	}

	if len(templates) == 0 {
		return nil, errors.New("no disk templates found")
	}

	return templates, nil
}

func testAccCheckKatapultDiskTemplates(
	res string,
	tpls []*core.DiskTemplate,
) resource.TestCheckFunc {
	tfs := []resource.TestCheckFunc{}

	for i, tpl := range tpls {
		prefix := fmt.Sprintf("templates.%d.", i)
		tfs = append(tfs,
			testAccCheckKatapultDiskTemplateAttrs(res, tpl, prefix),
		)
	}

	return resource.ComposeAggregateTestCheckFunc(tfs...)
}
