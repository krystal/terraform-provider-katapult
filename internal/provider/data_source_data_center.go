package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/krystal/go-katapult/core"
)

func dataSourceDataCenter() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceDataCenterRead,
		Schema: map[string]*schema.Schema{
			"id": {
				Type:        schema.TypeString,
				Computed:    true,
				Optional:    true,
				Description: "The ID of this resource.",
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

	var dc *core.DataCenter
	var err error

	switch {
	case id != "":
		dc, _, err = m.Core.DataCenters.GetByID(ctx, id)
	case permalink != "":
		dc, _, err = m.Core.DataCenters.GetByPermalink(ctx, permalink)
	default:
		dc, _, err = m.Core.DataCenters.Get(ctx, m.DataCenterRef)
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
