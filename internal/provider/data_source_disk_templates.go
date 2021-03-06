package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/krystal/go-katapult/core"
)

func dataSourceDiskTemplates() *schema.Resource {
	dts := dataSourceSchemaFromResourceSchema(dataSourceDiskTemplate().Schema)

	return &schema.Resource{
		ReadContext: dataSourceDiskTemplatesRead,
		Schema: map[string]*schema.Schema{
			"id": {
				Description: "Always set to provider organization value.",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"include_universal": {
				Type:        schema.TypeBool,
				Description: "Include universal disk templates.",
				Optional:    true,
				Default:     true,
			},
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
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)
	var diags diag.Diagnostics

	universal := d.Get("include_universal").(bool)

	var templates []*core.DiskTemplate
	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		pageResult, resp, err := m.Core.DiskTemplates.List(
			ctx, m.OrganizationRef, &core.DiskTemplateListOptions{
				IncludeUniversal: universal,
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

	d.SetId(m.confOrganization)

	return diags
}

func flattenDiskTemplates(
	tpls []*core.DiskTemplate,
) []map[string]interface{} {
	r := make([]map[string]interface{}, 0, len(tpls))

	for _, tpl := range tpls {
		r = append(r, flattenDiskTemplate(tpl))
	}

	return r
}
