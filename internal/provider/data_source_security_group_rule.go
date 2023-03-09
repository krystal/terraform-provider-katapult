package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceSecurityGroupRule() *schema.Resource {
	ds := dataSourceSchemaFromResourceSchema(resourceSecurityGroupRule().Schema)

	ds["id"] = &schema.Schema{
		Type:        schema.TypeString,
		Required:    true,
		Description: "The ID of this resource.",
	}

	return &schema.Resource{
		ReadContext: dataSourceSecurityGroupRuleRead,
		Schema:      ds,
		Description: "Fetch details for a individual security group rule.",
	}
}

func dataSourceSecurityGroupRuleRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)
	id := d.Get("id").(string)

	sgrID, err := sharedSecurityGroupRuleRead(ctx, d, m, id)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(sgrID)

	return diag.Diagnostics{}
}
