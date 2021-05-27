package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/krystal/go-katapult/core"
)

func resourceVirtualMachineGroup() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceVirtualMachineGroupCreate,
		ReadContext:   resourceVirtualMachineGroupRead,
		UpdateContext: resourceVirtualMachineGroupUpdate,
		DeleteContext: resourceVirtualMachineGroupDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"segregate": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  true,
			},
		},
	}
}

func resourceVirtualMachineGroupCreate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)
	segregate := d.Get("segregate").(bool)
	args := &core.VirtualMachineGroupCreateArguments{
		Name:      d.Get("name").(string),
		Segregate: &segregate,
	}

	vmg, _, err := m.Core.VirtualMachineGroups.Create(
		ctx, m.OrganizationRef, args,
	)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(vmg.ID)

	return resourceVirtualMachineGroupRead(ctx, d, meta)
}

func resourceVirtualMachineGroupRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)
	var diags diag.Diagnostics

	vmg, resp, err := m.Core.VirtualMachineGroups.GetByID(ctx, d.Id())
	if err != nil {
		if resp != nil && resp.Response != nil && resp.StatusCode == 404 {
			d.SetId("")

			return diags
		}

		return diag.FromErr(err)
	}

	_ = d.Set("name", vmg.Name)
	_ = d.Set("segregate", vmg.Segregate)

	return diags
}

func resourceVirtualMachineGroupUpdate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)

	vmGroupRef := core.VirtualMachineGroupRef{ID: d.Id()}
	args := &core.VirtualMachineGroupUpdateArguments{}

	if d.HasChange("name") {
		args.Name = d.Get("name").(string)
	}
	if d.HasChange("segregate") {
		segregate := d.Get("segregate").(bool)
		args.Segregate = &segregate
	}

	_, _, err := m.Core.VirtualMachineGroups.Update(ctx, vmGroupRef, args)
	if err != nil {
		return diag.FromErr(err)
	}

	return resourceVirtualMachineGroupRead(ctx, d, meta)
}

func resourceVirtualMachineGroupDelete(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)

	vmGroupRef := core.VirtualMachineGroupRef{ID: d.Id()}
	_, err := m.Core.VirtualMachineGroups.Delete(ctx, vmGroupRef)
	if err != nil {
		return diag.FromErr(err)
	}

	return diag.Diagnostics{}
}
