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
				Type:         schema.TypeString,
				Computed:     true,
				Optional:     true,
				AtLeastOneOf: []string{"id", "permalink"},
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
				Computed: true,
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
		},
	}
}

func dataSourceDiskTemplateRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)
	var diags diag.Diagnostics

	id := d.Get("id").(string)
	permalink := d.Get("permalink").(string)

	var template *katapult.DiskTemplate
	var err error

	switch {
	case id != "":
		template, _, err = m.Client.DiskTemplates.GetByID(ctx, id)
	case permalink != "":
		template, _, err = m.Client.DiskTemplates.GetByPermalink(ctx, permalink)
	}
	if err != nil {
		return diag.FromErr(err)
	}

	if template != nil {
		dt := flattenDiskTemplate(template)
		_ = d.Set("id", dt["id"])
		_ = d.Set("name", dt["name"])
		_ = d.Set("description", dt["description"])
		_ = d.Set("permalink", dt["permalink"])
		_ = d.Set("universal", dt["universal"])

		if v, ok := dt["template_version"]; ok {
			_ = d.Set("template_version", v)
		}

		if v, ok := dt["os_family"]; ok {
			_ = d.Set("os_family", v)
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
