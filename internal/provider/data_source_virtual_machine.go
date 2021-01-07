package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/krystal/go-katapult/pkg/katapult"
)

func dataSourceVirtualMachine() *schema.Resource {
	ds := dataSourceSchemaFromResourceSchema(resourceVirtualMachine().Schema)

	ds["id"] = &schema.Schema{
		Type:         schema.TypeString,
		Optional:     true,
		AtLeastOneOf: []string{"id", "fqdn"},
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
	m interface{},
) diag.Diagnostics {
	meta := m.(*Meta)
	var diags diag.Diagnostics

	id := d.Get("id").(string)
	fqdn := d.Get("fqdn").(string)

	var vm *katapult.VirtualMachine
	var err error

	switch {
	case id != "":
		vm, _, err = meta.Client.VirtualMachines.GetByID(ctx, id)
	case fqdn != "":
		vm, _, err = meta.Client.VirtualMachines.GetByFQDN(ctx, fqdn)
	}
	if err != nil {
		return diag.FromErr(err)
	}

	_ = d.Set("name", vm.Name)
	_ = d.Set("hostname", vm.Hostname)
	_ = d.Set("description", vm.Description)
	_ = d.Set("fqdn", vm.FQDN)
	_ = d.Set("state", vm.State)

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

	err = d.Set("tags", flattenTagNames(vm.TagNames))
	if err != nil {
		diags = append(diags, diag.FromErr(err)...)
	}

	d.SetId(vm.ID)

	return diags
}
