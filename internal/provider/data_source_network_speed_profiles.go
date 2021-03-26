package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/krystal/go-katapult/pkg/katapult"
)

func dataSourceNetworkSpeedProfiles() *schema.Resource {
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
					Schema: map[string]*schema.Schema{
						"id": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"name": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"permalink": {
							Type:     schema.TypeString,
							Computed: true,
							Optional: true,
						},
						"upload_speed": {
							Type:     schema.TypeInt,
							Computed: true,
							Description: "Upload speed in Mbit. A value of " +
								"`0` means unrestricted.",
						},
						"download_speed": {
							Type:     schema.TypeInt,
							Computed: true,
							Description: "Download speed in Mbit.A  value of " +
								"`0` means unrestricted.",
						},
					},
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

	var profiles []*katapult.NetworkSpeedProfile
	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		pageResult, resp, err := m.Client.NetworkSpeedProfiles.List(
			ctx, m.OrganizationRef(), &katapult.ListOptions{Page: pageNum},
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
	profiles []*katapult.NetworkSpeedProfile,
) []map[string]interface{} {
	f := make([]map[string]interface{}, 0, len(profiles))

	for _, profile := range profiles {
		f = append(f, flattenNetworkSpeedProfile(profile))
	}

	return f
}

func flattenNetworkSpeedProfile(
	profile *katapult.NetworkSpeedProfile,
) map[string]interface{} {
	f := make(map[string]interface{})

	f["id"] = profile.ID
	f["name"] = profile.Name
	f["permalink"] = profile.Permalink
	f["upload_speed"] = profile.UploadSpeedInMbit
	f["download_speed"] = profile.DownloadSpeedInMbit

	return f
}
