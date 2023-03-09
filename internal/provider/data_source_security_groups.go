package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/krystal/go-katapult/core"
)

func dataSourceSecurityGroups() *schema.Resource {
	sg := dataSourceSchemaFromResourceSchema(dataSourceSecurityGroup().Schema)

	return &schema.Resource{
		ReadContext: dataSourceSecurityGroupsRead,
		Description: "Fetch all security groups in the organization, " +
			"optionally including all rules for each security group.",
		Schema: map[string]*schema.Schema{
			"include_rules": {
				Type: schema.TypeBool,
				Description: "Whether to include rules in the output. Can " +
					"be slow if there are a lot of security groups, as each" +
					"group requires a separate API call to fetch rules.",
				Optional: true,
				Default:  false,
			},
			"security_groups": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: sg,
				},
			},
		},
	}
}

func dataSourceSecurityGroupsRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)
	var diags diag.Diagnostics

	orgRef := m.OrganizationRef
	includeRules := d.Get("include_rules").(bool)

	groups, err := getAllFlattenedSecurityGroups(ctx, m, orgRef, includeRules)
	if err != nil {
		return diag.FromErr(err)
	}

	err = d.Set("security_groups", groups)
	m.Logger.Debug("set security groups", "groups", groups, "err", err)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(m.confOrganization)

	return diags
}

func getAllFlattenedSecurityGroups(
	ctx context.Context,
	m *Meta,
	orgRef core.OrganizationRef,
	includeRules bool,
) ([]map[string]any, error) {
	var groups []map[string]any
	totalPages := 2

	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		pageResult, resp, err := m.Core.SecurityGroups.List(
			ctx, orgRef, &core.ListOptions{Page: pageNum},
		)
		if err != nil {
			return nil, err
		}

		totalPages = resp.Pagination.TotalPages

		for _, sg := range pageResult {
			flattened := flattenSecurityGroup(sg)
			if includeRules {
				inbound, outbound, err := getAllFlattenedSecurityGroupRules(
					ctx, m, sg.Ref(),
				)
				if err != nil {
					return nil, err
				}

				flattened["inbound_rules"] = inbound
				flattened["outbound_rules"] = outbound
			}

			groups = append(groups, flattened)
		}
	}

	return groups, nil
}

func flattenSecurityGroup(sg *core.SecurityGroup) map[string]any {
	return map[string]any{
		"id":                 sg.ID,
		"name":               sg.Name,
		"allow_all_inbound":  sg.AllowAllInbound,
		"allow_all_outbound": sg.AllowAllOutbound,
		"associations":       stringSliceToSchemaSet(sg.Associations),
	}
}
