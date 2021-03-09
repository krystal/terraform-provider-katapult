package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

//nolint:deadcode,unused
func dataSourceLoadBalancer() *schema.Resource {
	ds := dataSourceSchemaFromResourceSchema(resourceLoadBalancer().Schema)

	ds["id"] = &schema.Schema{
		Type:     schema.TypeString,
		Required: true,
	}

	return &schema.Resource{
		ReadContext: dataSourceLoadBalancerRead,
		Schema:      ds,
	}
}

//nolint:unused
func dataSourceLoadBalancerRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)
	var diags diag.Diagnostics

	id := d.Get("id").(string)

	lb, _, err := m.Client.LoadBalancers.GetByID(ctx, id)
	if err != nil {
		return diag.FromErr(err)
	}

	_ = d.Set("name", lb.Name)
	_ = d.Set("resource_type", string(lb.ResourceType))
	populateLoadBalancerTargets(d, lb.ResourceType, lb.ResourceIDs)
	_ = d.Set("https_redirect", lb.HTTPSRedirect)

	if lb.IPAddress != nil {
		_ = d.Set("ip_address", lb.IPAddress.Address)
	}

	d.SetId(lb.ID)

	return diags
}
