package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/krystal/go-katapult/pkg/katapult"
)

func dataSourceVirtualMachinePackages() *schema.Resource {
	ps := dataSourceSchemaFromResourceSchema(
		dataSourceVirtualMachinePackage().Schema,
	)

	return &schema.Resource{
		Description: "Fetch details of all Virtual Machine Packages",
		ReadContext: dataSourceVirtualMachinePackagesRead,
		Schema: map[string]*schema.Schema{
			"id": {
				Description: "Always set to `all`.",
				Type:        schema.TypeString,
				Computed:    true,
			},
			"packages": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: ps,
				},
			},
		},
	}
}

func dataSourceVirtualMachinePackagesRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)
	var diags diag.Diagnostics

	var pkgs []*katapult.VirtualMachinePackage
	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		pageResult, resp, err := m.Client.VirtualMachinePackages.List(
			ctx, &katapult.ListOptions{Page: pageNum},
		)
		if err != nil {
			return diag.FromErr(err)
		}

		totalPages = resp.Pagination.TotalPages
		pkgs = append(pkgs, pageResult...)
	}

	f := flattenVirtualMachinePackages(pkgs)
	if err := d.Set("packages", f); err != nil {
		return diag.FromErr(err)
	}

	d.SetId("all")

	return diags
}

func flattenVirtualMachinePackages(
	pkgs []*katapult.VirtualMachinePackage,
) []map[string]interface{} {
	r := make([]map[string]interface{}, 0, len(pkgs))

	for _, pkg := range pkgs {
		r = append(r, flattenVirtualMachinePackage(pkg))
	}

	return r
}
