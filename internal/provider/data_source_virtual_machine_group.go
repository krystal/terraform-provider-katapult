package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceVirtualMachineGroup() *schema.Resource {
	ds := dataSourceSchemaFromResourceSchema(resourceVirtualMachineGroup().Schema)

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

	vmg, _, err := m.Client.VirtualMachineGroups.GetByID(ctx, id)

	if err != nil {
		return diag.FromErr(err)
	}

	_ = d.Set("name", vmg.Name)
	_ = d.Set("segregate", vmg.Segregate)

	d.SetId(vmg.ID)

	return diags
}
