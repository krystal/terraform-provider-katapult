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

	// Remove creation-only fields which cannot be read back from the API.
	delete(ds, "disk")

	return &schema.Resource{
		ReadContext: dataSourceVirtualMachineRead,
		Schema:      ds,
	}
}

//nolint:funlen,gocyclo
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

	ifaces, err := nextFetchAllVMNetworkInterfaces(ctx, m, vm.ID)
	if err != nil {
		return append(diags, diag.FromErr(err)...)
	}

	virtualNetworkIDs := make([]string, 0, len(ifaces))
	for _, iface := range ifaces {
		if iface.VirtualNetwork.IsSpecified() {
			vnet, err2 := iface.VirtualNetwork.Get()
			if err2 != nil {
				continue
			}

			if *iface.State != "attached" {
				continue
			}

			if id := *vnet.Id; id != "" {
				virtualNetworkIDs = append(virtualNetworkIDs, id)
			}
		}
	}

	// As we set the speed profile for all interfaces on a VM, we only care
	// about fetching details about any single interface.
	var nsp string
	if len(ifaces) > 0 {
		vmnet, _, err2 := m.Core.VirtualMachineNetworkInterfaces.GetByID(
			ctx, *ifaces[0].Id,
		)
		if err2 != nil {
			return append(diags, diag.FromErr(err2)...)
		}

		if vmnet.SpeedProfile != nil {
			nsp = vmnet.SpeedProfile.Permalink
		}
	}

	_ = d.Set("name", vm.Name)
	_ = d.Set("hostname", vm.Hostname)
	_ = d.Set("description", vm.Description)
	_ = d.Set("fqdn", vm.FQDN)
	_ = d.Set("state", vm.State)

	if nsp != "" {
		_ = d.Set("network_speed_profile", nsp)
	}

	if grp := vm.Group; grp != nil {
		_ = d.Set("group_id", grp.ID)
	}

	if pkg := normalizeVirtualMachinePackage(vm.Package); pkg != "" {
		_ = d.Set("package", pkg)
	}

	err = d.Set(
		"ip_address_ids",
		stringSliceToSchemaSet(flattenIPAddressIDs(vm.IPAddresses)),
	)
	if err != nil {
		diags = append(diags, diag.FromErr(err)...)
	}

	err = d.Set(
		"ip_addresses",
		stringSliceToSchemaSet(flattenIPAddresses(vm.IPAddresses)),
	)
	if err != nil {
		diags = append(diags, diag.FromErr(err)...)
	}

	err = d.Set(
		"virtual_network_ids",
		stringSliceToSchemaSet(virtualNetworkIDs),
	)
	if err != nil {
		diags = append(diags, diag.FromErr(err)...)
	}

	err = d.Set("network_interfaces", flattenNetworkInterfaces(ifaces))
	if err != nil {
		diags = append(diags, diag.FromErr(err)...)
	}

	err = d.Set("tags", stringSliceToSchemaSet(vm.TagNames))
	if err != nil {
		diags = append(diags, diag.FromErr(err)...)
	}

	d.SetId(vm.ID)

	return diags
}
