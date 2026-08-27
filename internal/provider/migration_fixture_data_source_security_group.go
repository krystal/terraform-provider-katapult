package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceSecurityGroup() *schema.Resource {
	ds := dataSourceSchemaFromResourceSchema(resourceSecurityGroup().Schema)

	ds["id"] = &schema.Schema{
		Type:        schema.TypeString,
		Required:    true,
		Description: "The ID of this resource.",
	}
	ds["include_rules"] = &schema.Schema{
		Type:        schema.TypeBool,
		Description: "Whether to include rules in the output.",
		Optional:    true,
		Default:     true,
	}

	delete(ds, "external_rules")

	ds["inbound_rules"] = ds["inbound_rule"]
	delete(ds, "inbound_rule")

	ds["outbound_rules"] = ds["outbound_rule"]
	delete(ds, "outbound_rule")

	return &schema.Resource{
		ReadContext: dataSourceSecurityGroupRead,
		Schema:      ds,
		Description: "Fetch details for a individual security group, " +
			"including all rules.",
	}
}

func dataSourceSecurityGroupRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)
	var diags diag.Diagnostics

	sg, _, err := m.Core.SecurityGroups.GetByID(ctx, d.Get("id").(string))
	if err != nil {
		return diag.FromErr(err)
	}

	_ = d.Set("name", sg.Name)
	_ = d.Set("allow_all_inbound", sg.AllowAllInbound)
	_ = d.Set("allow_all_outbound", sg.AllowAllOutbound)

	err = d.Set("associations", stringSliceToSchemaSet(sg.Associations))
	if err != nil {
		diags = append(diags, diag.FromErr(err)...)
	}

	d.SetId(sg.ID)

	// Skip fetching rules if include_rules is false.
	if !d.Get("include_rules").(bool) {
		return diags
	}

	inbound, outbound, err := getAllFlattenedSecurityGroupRules(
		ctx, m, sg.Ref(),
	)
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

	return diags
}
