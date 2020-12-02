package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/krystal/go-katapult/pkg/katapult"
)

func dataSourceDiskTemplates() *schema.Resource {
	dts := dataSourceDiskTemplate().Schema

	return &schema.Resource{
		ReadContext: dataSourceDiskTemplatesRead,
		Schema: map[string]*schema.Schema{
			"templates": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: dts,
				},
			},
		},
	}
}

func dataSourceDiskTemplatesRead(
	ctx context.Context,
	d *schema.ResourceData,
	m interface{},
) diag.Diagnostics {
	meta := m.(*Meta)
	c := meta.Client
	var diags diag.Diagnostics

	org := meta.Organization()

	var templates []*katapult.DiskTemplate
	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		pageResult, resp, err := c.DiskTemplates.List(
			ctx, org, &katapult.DiskTemplateListOptions{
				IncludeUniversal: true,
				Page:             pageNum,
			},
		)
		if err != nil {
			return diag.FromErr(err)
		}

		totalPages = resp.Pagination.TotalPages
		templates = append(templates, pageResult...)
	}

	dts := flattenDiskTemplates(templates)
	if err := d.Set("templates", dts); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(org.ID)

	return diags
}

func flattenDiskTemplates(
	tpls []*katapult.DiskTemplate,
) []map[string]interface{} {
	dts := make([]map[string]interface{}, 0)
	for _, tpl := range tpls {
		dts = append(dts, flattenDiskTemplate(tpl))
	}

	return dts
}
