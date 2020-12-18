package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/krystal/go-katapult/pkg/katapult"
)

func dataSourceIP() *schema.Resource {
	ds := dataSourceSchemaFromResourceSchema(resourceIP().Schema)

	ds["id"] = &schema.Schema{
		Type:         schema.TypeString,
		Optional:     true,
		AtLeastOneOf: []string{"id", "address"},
	}

	ds["address"].Optional = true

	return &schema.Resource{
		ReadContext: dataSourceIPRead,
		Schema:      ds,
	}
}

func dataSourceIPRead(
	ctx context.Context,
	d *schema.ResourceData,
	m interface{},
) diag.Diagnostics {
	meta := m.(*Meta)
	var diags diag.Diagnostics

	id := d.Get("id").(string)
	address := d.Get("address").(string)

	var ip *katapult.IPAddress
	var err error

	switch {
	case id != "":
		ip, _, err = meta.Client.IPAddresses.GetByID(ctx, id)
	default:
		ip, _, err = meta.Client.IPAddresses.GetByAddress(ctx, address)
	}

	if err != nil {
		return diag.FromErr(err)
	}

	if ip.Network != nil {
		_ = d.Set("network_id", ip.Network.ID)
	}
	_ = d.Set("address", ip.Address)
	_ = d.Set("address_with_mask", ip.AddressWithMask)
	_ = d.Set("reverse_dns", ip.ReverseDNS)
	_ = d.Set("version", flattenIPVersion(ip.Address))
	_ = d.Set("vip", ip.VIP)
	_ = d.Set("label", ip.Label)
	_ = d.Set("allocation_type", ip.AllocationType)
	_ = d.Set("allocation_id", ip.AllocationID)

	d.SetId(ip.ID)

	return diags
}
