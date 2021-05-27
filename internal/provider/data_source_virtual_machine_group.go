package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/krystal/go-katapult/core"
)

func dataSourceVirtualMachineGroup() *schema.Resource {
	ds := dataSourceSchemaFromResourceSchema(
		resourceVirtualMachineGroup().Schema,
	)

	ds["id"] = &schema.Schema{
		Type:     schema.TypeString,
		Required: true,
	}

	return &schema.Resource{
		ReadContext: dataSourceVirtualMachineGroupRead,
		Schema:      ds,
	}
}

func dataSourceVirtualMachineGroupRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)
	var diags diag.Diagnostics

	id := d.Get("id").(string)

	vmg, _, err := m.Core.VirtualMachineGroups.GetByID(ctx, id)
	if err != nil {
		return diag.FromErr(err)
	}

	f := flattenVirtualMachineGroup(vmg)

	_ = d.Set("name", f["name"])
	_ = d.Set("segregate", f["segregate"])

	d.SetId(vmg.ID)

	return diags
}

func flattenVirtualMachineGroup(
	pkg *core.VirtualMachineGroup,
) map[string]interface{} {
	r := make(map[string]interface{})

	r["id"] = pkg.ID
	r["name"] = pkg.Name
	r["segregate"] = pkg.Segregate

	return r
}
