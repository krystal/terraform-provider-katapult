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
				Computed: true,
				Optional: true,
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
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)
	var diags diag.Diagnostics

	id := d.Get("id").(string)
	permalink := d.Get("permalink").(string)

	var dc *katapult.DataCenter
	var err error

	switch {
	case id != "":
		dc, _, err = m.Client.DataCenters.GetByID(ctx, id)
	case permalink != "":
		dc, _, err = m.Client.DataCenters.GetByPermalink(ctx, permalink)
	default:
		dc, err = m.DataCenter(ctx)
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
	m *Meta,
) (*katapult.Network, error) {
	networks, _, _, err := m.Client.Networks.List(
		ctx, m.OrganizationRef(),
	)
	if err != nil {
		return nil, err
	}

	dcID, err := m.DataCenterID(ctx)
	if err != nil {
		return nil, err
	}

	for _, network := range networks {
		if network.DataCenter != nil && network.DataCenter.ID == dcID &&
			strings.Contains(strings.ToLower(network.Name), "public") {
			return network, nil
		}
	}

	return nil, fmt.Errorf(
		"default network for data center %s could not be determined", dcID,
	)
}
