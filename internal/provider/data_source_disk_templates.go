package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/krystal/go-katapult/pkg/katapult"
)

func dataSourceDiskTemplates() *schema.Resource {
	dts := dataSourceSchemaFromResourceSchema(dataSourceDiskTemplate().Schema)

	return &schema.Resource{
		ReadContext: dataSourceDiskTemplatesRead,
		Schema: map[string]*schema.Schema{
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

	orgID, err := m.OrganizationID(ctx)
	if err != nil {
		return diag.FromErr(err)
	}

	universal := d.Get("include_universal").(bool)

	var templates []*katapult.DiskTemplate
	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		pageResult, resp, err := m.Client.DiskTemplates.List(
			ctx, m.OrganizationRef(), &katapult.DiskTemplateListOptions{
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

	d.SetId(orgID)

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
