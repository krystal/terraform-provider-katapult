package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/krystal/go-katapult/pkg/katapult"
)

func dataSourceVirtualMachineGroups() *schema.Resource {
	ds := dataSourceSchemaFromResourceSchema(
		resourceVirtualMachineGroup().Schema,
	)

	return &schema.Resource{
		ReadContext: dataSourceVirtualMachineGroupsRead,
		Schema: map[string]*schema.Schema{
			"id": {
				Description: "Always set to provider organization value.",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"groups": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: ds,
				},
			},
		},
	}
}

func dataSourceVirtualMachineGroupsRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)
	var diags diag.Diagnostics

	groups, _, err := m.Client.VirtualMachineGroups.List(
		ctx, m.OrganizationRef(),
	)
	if err != nil {
		return diag.FromErr(err)
	}

	f := flattenVirtualMachineGroups(groups)
	if err := d.Set("groups", f); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(m.confOrganization)

	return diags
}

func flattenVirtualMachineGroups(
	groups []*katapult.VirtualMachineGroup,
) []map[string]interface{} {
	r := make([]map[string]interface{}, 0, len(groups))

	for _, group := range groups {
		r = append(r, flattenVirtualMachineGroup(group))
	}

	return r
}
