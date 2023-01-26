package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/krystal/go-katapult/core"
)

func dataSourceVirtualMachine() *schema.Resource {
	ds := dataSourceSchemaFromResourceSchema(resourceVirtualMachine().Schema)

	ds["id"] = &schema.Schema{
		Type:         schema.TypeString,
		Optional:     true,
		AtLeastOneOf: []string{"id", "fqdn"},
		Description:  "The ID of this resource.",
	}

	ds["fqdn"].Optional = true

	return &schema.Resource{
		ReadContext: dataSourceVirtualMachineRead,
		Schema:      ds,
	}
}

func dataSourceVirtualMachineRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)
	var diags diag.Diagnostics

	id := d.Get("id").(string)
	fqdn := d.Get("fqdn").(string)

	var vm *core.VirtualMachine
	var err error

	switch {
	case id != "":
		vm, _, err = m.Core.VirtualMachines.GetByID(ctx, id)
	case fqdn != "":
		vm, _, err = m.Core.VirtualMachines.GetByFQDN(ctx, fqdn)
	}
	if err != nil {
		return diag.FromErr(err)
	}

	_ = d.Set("name", vm.Name)
	_ = d.Set("hostname", vm.Hostname)
	_ = d.Set("description", vm.Description)
	_ = d.Set("fqdn", vm.FQDN)
	_ = d.Set("state", vm.State)

	if grp := vm.Group; grp != nil {
		_ = d.Set("group_id", grp.ID)
	}

	if pkg := normalizeVirtualMachinePackage(vm.Package); pkg != "" {
		_ = d.Set("package", pkg)
	}

	err = d.Set(
		"ip_address_ids",
		newSchemaStringSet(flattenIPAddressIDs(vm.IPAddresses)),
	)
	if err != nil {
		diags = append(diags, diag.FromErr(err)...)
	}

	err = d.Set(
		"ip_addresses",
		newSchemaStringSet(flattenIPAddresses(vm.IPAddresses)),
	)
	if err != nil {
		diags = append(diags, diag.FromErr(err)...)
	}

	// As we set the speed profile for all interfaces on a VM, we only care
	// about fetching details about any single interface.
	vmnets, _, err := m.Core.VirtualMachineNetworkInterfaces.List(
		ctx, vm.Ref(), &core.ListOptions{Page: 1, PerPage: 1},
	)
	if err != nil {
		diags = append(diags, diag.FromErr(err)...)
	} else if len(vmnets) > 0 {
		vmnet, _, err2 := m.Core.VirtualMachineNetworkInterfaces.Get(
			ctx, vmnets[0].Ref(),
		)
		if err2 != nil {
			diags = append(diags, diag.FromErr(err2)...)
		} else if vmnet.SpeedProfile != nil {
			_ = d.Set("network_speed_profile", vmnet.SpeedProfile.Permalink)
		}
	}

	err = d.Set("tags", flattenTagNames(vm.TagNames))
	if err != nil {
		diags = append(diags, diag.FromErr(err)...)
	}

	d.SetId(vm.ID)

	return diags
}
