package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/krystal/go-katapult/pkg/katapult"
)

func dataSourceDiskTemplate() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceDiskTemplateRead,
		Schema: map[string]*schema.Schema{
			"id": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"name": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"description": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"permalink": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"universal": {
				Type:     schema.TypeBool,
				Computed: true,
			},
			"template_version": {
				Type:     schema.TypeInt,
				Computed: true,
			},
			"os_family": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"organization_id": {
				Type:     schema.TypeString,
				Optional: true,
			},
		},
	}
}

func dataSourceDiskTemplateRead(
	ctx context.Context,
	d *schema.ResourceData,
	m interface{},
) diag.Diagnostics {
	meta := m.(*Meta)
	c := meta.Client
	var diags diag.Diagnostics

	id := d.Get("id").(string)
	permalink := d.Get("permalink").(string)

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

	var template *katapult.DiskTemplate
	for _, t := range templates {
		if (id != "" && id == t.ID) ||
			(permalink != "" && permalink == t.Permalink) {
			template = t

			break
		}
	}

	if template != nil {
		dt := flattenDiskTemplate(template)
		for key, value := range dt {
			_ = d.Set(key, value)
		}

		d.SetId(template.ID)
	}

	return diags
}

func flattenDiskTemplate(tpl *katapult.DiskTemplate) map[string]interface{} {
	dt := make(map[string]interface{})

	dt["id"] = tpl.ID
	dt["name"] = tpl.Name
	dt["description"] = tpl.Description
	dt["permalink"] = tpl.Permalink
	dt["universal"] = tpl.Universal

	if tpl.LatestVersion != nil {
		dt["template_version"] = tpl.LatestVersion.Number
	}

	if tpl.OperatingSystem != nil {
		dt["os_family"] = tpl.OperatingSystem.Name
	}

	return dt
}
