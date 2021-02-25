package provider

import (
	"errors"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/pkg/katapult"
	"github.com/stretchr/testify/require"
)

func TestAccKatapultDataSourceDiskTemplates_default(t *testing.T) {
	tt := NewTestTools(t)
	defer tt.Cleanup()

	tpls, err := testFetchAllDiskTemplates(tt)
	require.NoError(t, err)

	res := "data.katapult_disk_templates.main"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "katapult_disk_templates" "main" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultDiskTemplates(tt, res, tpls),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceDiskTemplates_include_universal(t *testing.T) {
	tt := NewTestTools(t)
	defer tt.Cleanup()

	tpls, err := testFetchAllDiskTemplates(tt)
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
						tt, "data.katapult_disk_templates.main", tpls,
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceDiskTemplates_exclude_universal(t *testing.T) {
	tt := NewTestTools(t)
	defer tt.Cleanup()

	allTpls, err := testFetchAllDiskTemplates(tt)
	require.NoError(t, err)

	tpls := []*katapult.DiskTemplate{}
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
						tt, "data.katapult_disk_templates.main", tpls,
					),
				),
			},
		},
	})
}

func testFetchAllDiskTemplates(
	tt *TestTools,
) ([]*katapult.DiskTemplate, error) {
	var templates []*katapult.DiskTemplate
	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		pageResult, resp, err := tt.Meta.Client.DiskTemplates.List(
			tt.Meta.Ctx, tt.Meta.OrganizationRef(),
			&katapult.DiskTemplateListOptions{
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
	tt *TestTools,
	res string,
	tpls []*katapult.DiskTemplate,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		for i, tpl := range tpls {
			prefix := fmt.Sprintf("templates.%d.", i)
			err := testAccCheckKatapultDiskTemplate(tt, res, prefix, tpl)(s)
			if err != nil {
				return err
			}
		}

		return nil
	}
}
