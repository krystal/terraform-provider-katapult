package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/pkg/katapult"
	"github.com/stretchr/testify/require"
)

func TestAccKatapultDataSourceDiskTemplate_by_id(t *testing.T) {
	tt := NewTestTools(t)
	defer tt.Cleanup()

	tpl, err := testHelperFetchRandomDiskTemplate(tt)
	require.NoError(t, err)

	res := "data.katapult_disk_template.main"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					data "katapult_disk_template" "main" {
					  id = "%s"
					}`,
					tpl.ID,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultDiskTemplateAttrs(tt, res, "", tpl),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceDiskTemplate_by_permalink(t *testing.T) {
	tt := NewTestTools(t)
	defer tt.Cleanup()

	tpl, err := testHelperFetchRandomDiskTemplate(tt)
	require.NoError(t, err)

	res := "data.katapult_disk_template.main"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					data "katapult_disk_template" "main" {
					  permalink = "%s"
					}`,
					tpl.Permalink,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultDiskTemplateAttrs(tt, res, "", tpl),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceDiskTemplate_blank(t *testing.T) {
	tt := NewTestTools(t)
	defer tt.Cleanup()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "katapult_disk_template" "main" {}`,
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta("one of `id,permalink` must be specified"),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceDiskTemplate_invalid(t *testing.T) {
	tt := NewTestTools(t)
	defer tt.Cleanup()

	tpl, err := testHelperFetchRandomDiskTemplate(tt)
	require.NoError(t, err)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					data "katapult_disk_template" "main" {
					  name = "%s"
					}`,
					tpl.Name,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta("Computed attributes cannot be set"),
				),
			},
		},
	})
}

//
// Helpers
//

func testHelperFetchRandomDiskTemplate(
	tt *TestTools,
) (*katapult.DiskTemplate, error) {
	templates, err := testHelperFetchAllDiskTemplates(tt)
	if err != nil {
		return nil, err
	}

	return templates[acctest.RandIntRange(0, len(templates))], nil
}

func testAccCheckKatapultDiskTemplateAttrs(
	tt *TestTools,
	res string,
	prefix string,
	tpl *katapult.DiskTemplate,
) resource.TestCheckFunc {
	tfs := []resource.TestCheckFunc{
		resource.TestCheckResourceAttr(res, prefix+"id", tpl.ID),
		resource.TestCheckResourceAttr(res, prefix+"name", tpl.Name),
		resource.TestCheckResourceAttr(
			res, prefix+"description", tpl.Description,
		),
		resource.TestCheckResourceAttr(
			res, prefix+"permalink", tpl.Permalink,
		),
		resource.TestCheckResourceAttr(
			res, prefix+"universal", fmt.Sprintf("%t", tpl.Universal),
		),
	}

	if tpl.LatestVersion != nil {
		tfs = append(tfs, resource.TestCheckResourceAttr(
			res, prefix+"template_version",
			fmt.Sprintf("%d", tpl.LatestVersion.Number),
		))
	}

	if tpl.OperatingSystem != nil {
		tfs = append(tfs, resource.TestCheckResourceAttr(
			res, prefix+"os_family", tpl.OperatingSystem.Name,
		))
	}

	return resource.ComposeAggregateTestCheckFunc(tfs...)
}
