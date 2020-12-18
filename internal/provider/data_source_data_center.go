package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/krystal/go-katapult/pkg/katapult"
)

func dataSourceDataCenter() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceDataCenterRead,
		Schema: map[string]*schema.Schema{
			"id": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"name": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"permalink": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"country_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"country_name": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func dataSourceDataCenterRead(
	ctx context.Context,
	d *schema.ResourceData,
	m interface{},
) diag.Diagnostics {
	meta := m.(*Meta)
	c := meta.Client
	var diags diag.Diagnostics

	id := d.Get("id").(string)
	permalink := d.Get("permalink").(string)

	var dc *katapult.DataCenter
	var err error

	switch {
	case id != "":
		dc, _, err = c.DataCenters.GetByID(ctx, id)
	case permalink != "":
		dc, _, err = c.DataCenters.GetByPermalink(ctx, permalink)
	case meta.DataCenterID != "":
		dc, _, err = c.DataCenters.GetByID(ctx, meta.DataCenterID)
	default:
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "\"id\" or \"permalink\" argument must be specified.",
		})

		return diags
	}

	if err != nil {
		return diag.FromErr(err)
	}

	_ = d.Set("name", dc.Name)
	_ = d.Set("permalink", dc.Permalink)
	if dc.Country != nil {
		_ = d.Set("country_id", dc.Country.ID)
		_ = d.Set("country_name", dc.Country.Name)
	}

	d.SetId(dc.ID)

	return diags
}

func defaultNetworkForDataCenter(
	ctx context.Context,
	meta *Meta,
	dc *katapult.DataCenter,
) (*katapult.Network, error) {
	networks, _, _, err := meta.Client.Networks.List(ctx, meta.Organization())
	if err != nil {
		return nil, err
	}

	if dc == nil {
		dc = meta.DataCenter()
	}

	for _, network := range networks {
		if network.DataCenter != nil && network.DataCenter.ID == dc.ID &&
			strings.Contains(strings.ToLower(network.Name), "public") {
			return network, nil
		}
	}

	return nil, fmt.Errorf(
		"default network for data center %s could not be determined", dc.ID,
	)
}
