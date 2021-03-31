package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/krystal/go-katapult/pkg/katapult"
)

func dataSourceNetworkSpeedProfile() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceNetworkSpeedProfileRead,
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
				Description: "Download speed in Mbit. A  value of " +
					"`0` means unrestricted.",
			},
		},
	}
}

func dataSourceNetworkSpeedProfileRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)
	var diags diag.Diagnostics

	id := d.Get("id").(string)
	permalink := d.Get("permalink").(string)

	profile, err := fetchNetworkSpeedProfile(ctx, m, id, permalink)
	if err != nil {
		return diag.FromErr(err)
	}

	if profile != nil {
		f := flattenNetworkSpeedProfile(profile)
		_ = d.Set("name", f["name"])
		_ = d.Set("permalink", f["permalink"])
		_ = d.Set("upload_speed", f["upload_speed"])
		_ = d.Set("download_speed", f["download_speed"])

		d.SetId(profile.ID)
	}

	return diags
}

func fetchNetworkSpeedProfile(
	ctx context.Context,
	m *Meta,
	id string,
	permalink string,
) (*katapult.NetworkSpeedProfile, error) {
	totalPages := 2
	for pageNum := 1; pageNum < totalPages; pageNum++ {
		profiles, resp, err := m.Client.NetworkSpeedProfiles.List(
			ctx, m.OrganizationRef(), &katapult.ListOptions{Page: pageNum},
		)
		if err != nil {
			return nil, err
		}

		totalPages = resp.Pagination.TotalPages
		for _, p := range profiles {
			if (id != "" && id == p.ID) ||
				(permalink != "" && permalink == p.Permalink) {
				return p, nil
			}
		}
	}

	return nil, nil
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
