package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/krystal/go-katapult/core"
)

func dataSourceVirtualMachinePackage() *schema.Resource {
	return &schema.Resource{
		Description: "Fetch details of a single Virtual Machine Package " +
			"using package `id` or `permalink`.",
		ReadContext: dataSourceVirtualMachinePackageRead,
		Schema: map[string]*schema.Schema{
			"id": {
				Type:         schema.TypeString,
				Computed:     true,
				Optional:     true,
				AtLeastOneOf: []string{"id", "permalink"},
			},
			"name": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"permalink": {
				Type:     schema.TypeString,
				Computed: true,
				Optional: true,
			},
			"cpu_cores": {
				Type:        schema.TypeInt,
				Description: "Number of CPU cores.",
				Computed:    true,
			},
			"ipv4_addresses": {
				Type:        schema.TypeInt,
				Description: "Number of included IPv4 addresses.",
				Computed:    true,
			},
			"memory_in_gb": {
				Type:        schema.TypeInt,
				Description: "Memory in GB.",
				Computed:    true,
			},
			"storage_in_gb": {
				Type:        schema.TypeInt,
				Description: "Storage in GB.",
				Computed:    true,
			},
			"privacy": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func dataSourceVirtualMachinePackageRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)
	var diags diag.Diagnostics

	id := d.Get("id").(string)
	permalink := d.Get("permalink").(string)

	var pkg *core.VirtualMachinePackage
	var err error

	switch {
	case id != "":
		pkg, _, err = m.Core.VirtualMachinePackages.GetByID(ctx, id)
	case permalink != "":
		pkg, _, err = m.Core.VirtualMachinePackages.GetByPermalink(
			ctx, permalink,
		)
	}
	if err != nil {
		return diag.FromErr(err)
	}

	if pkg != nil {
		f := flattenVirtualMachinePackage(pkg)

		_ = d.Set("id", f["id"])
		_ = d.Set("name", f["name"])
		_ = d.Set("permalink", f["permalink"])
		_ = d.Set("cpu_cores", f["cpu_cores"])
		_ = d.Set("ipv4_addresses", f["ipv4_addresses"])
		_ = d.Set("memory_in_gb", f["memory_in_gb"])
		_ = d.Set("storage_in_gb", f["storage_in_gb"])
		_ = d.Set("privacy", f["privacy"])

		d.SetId(pkg.ID)
	}

	return diags
}

func flattenVirtualMachinePackage(
	pkg *core.VirtualMachinePackage,
) map[string]interface{} {
	r := make(map[string]interface{})

	r["id"] = pkg.ID
	r["name"] = pkg.Name
	r["permalink"] = pkg.Permalink
	r["cpu_cores"] = pkg.CPUCores
	r["ipv4_addresses"] = pkg.IPv4Addresses
	r["memory_in_gb"] = pkg.MemoryInGB
	r["storage_in_gb"] = pkg.StorageInGB
	r["privacy"] = pkg.Privacy

	return r
}
