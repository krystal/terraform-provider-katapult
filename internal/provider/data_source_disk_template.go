package provider

import (
	"context"
	"strings"

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
	var diags diag.Diagnostics

	idOrPermalink := d.Get("id").(string)
	if idOrPermalink == "" {
		idOrPermalink = d.Get("permalink").(string)
	}

	template, err := fetchDiskTemplate(ctx, meta, idOrPermalink)
	if err != nil {
		return diag.FromErr(err)
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

func fetchDiskTemplate(
	ctx context.Context,
	meta *Meta,
	idOrPermalink string,
) (*katapult.DiskTemplate, error) {
	var id string
	var permalink string

	if strings.HasPrefix(idOrPermalink, "dtpl_") {
		id = idOrPermalink
	} else {
		permalink = idOrPermalink
	}

	org := meta.Organization()

	totalPages := 2
	for pageNum := 1; pageNum < totalPages; pageNum++ {
		templates, resp, err := meta.Client.DiskTemplates.List(
			ctx, org, &katapult.DiskTemplateListOptions{
				IncludeUniversal: true,
				Page:             pageNum,
			},
		)
		if err != nil {
			return nil, err
		}

		totalPages = resp.Pagination.TotalPages
		for _, t := range templates {
			if (id != "" && id == t.ID) ||
				(permalink != "" && permalink == t.Permalink) {
				return t, nil
			}
		}
	}

	return nil, nil
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
