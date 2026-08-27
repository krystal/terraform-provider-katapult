package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/krystal/go-katapult/core"
)

func dataSourceSecurityGroupRules() *schema.Resource {
	ruleSchema := dataSourceSchemaFromResourceSchema(
		resourceSecurityGroupRule().Schema,
	)
	delete(ruleSchema, "security_group_id")

	return &schema.Resource{
		ReadContext: dataSourceSecurityGroupRulesRead,
		Description: "Fetch all rules for a given security group.",
		Schema: map[string]*schema.Schema{
			"security_group_id": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "ID of Security Group to fetch rules for.",
			},
			"inbound_rules": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: ruleSchema,
				},
			},
			"outbound_rules": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: ruleSchema,
				},
			},
		},
	}
}

func dataSourceSecurityGroupRulesRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)
	var diags diag.Diagnostics

	id := d.Get("security_group_id").(string)
	sgRef := core.SecurityGroupRef{ID: id}

	inbound, outbound, err := getAllFlattenedSecurityGroupRules(ctx, m, sgRef)
	if err != nil {
		return diag.FromErr(err)
	}

	err = d.Set("inbound_rules", inbound)
	if err != nil {
		diags = append(diags, diag.FromErr(err)...)
	}

	err = d.Set("outbound_rules", outbound)
	if err != nil {
		diags = append(diags, diag.FromErr(err)...)
	}

	d.SetId(id)

	return diags
}
