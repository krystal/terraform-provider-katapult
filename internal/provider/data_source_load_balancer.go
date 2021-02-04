package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceLoadBalancer() *schema.Resource {
	ds := dataSourceSchemaFromResourceSchema(resourceLoadBalancer().Schema)

	ds["id"] = &schema.Schema{
		Type:     schema.TypeString,
		Computed: true,
		Optional: true,
	}

	return &schema.Resource{
		ReadContext: dataSourceLoadBalancerRead,
		Schema:      ds,
	}
}

func dataSourceLoadBalancerRead(
	ctx context.Context,
	d *schema.ResourceData,
	m interface{},
) diag.Diagnostics {
	meta := m.(*Meta)
	c := meta.Client
	var diags diag.Diagnostics

	id := d.Get("id").(string)

	lb, _, err := c.LoadBalancers.GetByID(ctx, id)
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
