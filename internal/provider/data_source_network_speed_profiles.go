package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/krystal/go-katapult/core"
)

func dataSourceNetworkSpeedProfiles() *schema.Resource {
	nsps := dataSourceSchemaFromResourceSchema(
		dataSourceNetworkSpeedProfile().Schema,
	)

	return &schema.Resource{
		ReadContext: dataSourceNetworkSpeedProfilesRead,
		Schema: map[string]*schema.Schema{
			"id": {
				Description: "Always set to provider organization value.",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"profiles": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: nsps,
				},
			},
		},
	}
}

func dataSourceNetworkSpeedProfilesRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)
	var diags diag.Diagnostics

	var profiles []*core.NetworkSpeedProfile
	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		pageResult, resp, err := m.Core.NetworkSpeedProfiles.List(
			ctx, m.OrganizationRef, &core.ListOptions{Page: pageNum},
		)
		if err != nil {
			return diag.FromErr(err)
		}

		totalPages = resp.Pagination.TotalPages
		profiles = append(profiles, pageResult...)
	}

	f := flattenNetworkSpeedProfiles(profiles)
	if err := d.Set("profiles", f); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(m.confOrganization)

	return diags
}

func flattenNetworkSpeedProfiles(
	profiles []*core.NetworkSpeedProfile,
) []map[string]interface{} {
	f := make([]map[string]interface{}, 0, len(profiles))

	for _, profile := range profiles {
		f = append(f, flattenNetworkSpeedProfile(profile))
	}

	return f
}
