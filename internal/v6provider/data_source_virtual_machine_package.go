package v6provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	core "github.com/krystal/go-katapult/next/core"
)

const virtualMachinePackagesPageSize = 100

var _ datasource.DataSourceWithConfigValidators = (*VirtualMachinePackageDataSource)(nil)

type (
	VirtualMachinePackageDataSource struct {
		M *Meta
	}

	VirtualMachinePackageDataSourceModel struct {
		ID            types.String `tfsdk:"id"`
		Name          types.String `tfsdk:"name"`
		Permalink     types.String `tfsdk:"permalink"`
		CPUCores      types.Int64  `tfsdk:"cpu_cores"`
		IPv4Addresses types.Int64  `tfsdk:"ipv4_addresses"`
		MemoryInGB    types.Int64  `tfsdk:"memory_in_gb"`
		StorageInGB   types.Int64  `tfsdk:"storage_in_gb"`
		Privacy       types.String `tfsdk:"privacy"`
	}
)

func (d *VirtualMachinePackageDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_virtual_machine_package"
}

func (d *VirtualMachinePackageDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}

	meta, ok := req.ProviderData.(*Meta)
	if !ok {
		resp.Diagnostics.AddError("Meta Error", "meta is not of type *Meta")
		return
	}

	d.M = meta
}

func virtualMachinePackageSchemaAttributes(selector bool) map[string]schema.Attribute {
	id := schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "The ID of this resource.",
	}
	permalink := schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: "The permalink of the virtual machine package.",
	}
	if selector {
		id.Optional = true
		permalink.Optional = true
	}

	return map[string]schema.Attribute{
		"id":        id,
		"permalink": permalink,
		"name": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "The name of the virtual machine package.",
		},
		"cpu_cores": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "Number of CPU cores.",
		},
		"ipv4_addresses": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "Number of included IPv4 addresses.",
		},
		"memory_in_gb": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "Memory in GB.",
		},
		"storage_in_gb": schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: "Storage in GB.",
		},
		"privacy": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "The visibility of the virtual machine package.",
		},
	}
}

func (d *VirtualMachinePackageDataSource) ConfigValidators(
	_ context.Context,
) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		nonEmptySelectorConfigValidator{},
	}
}

func (d *VirtualMachinePackageDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieves a virtual machine package by ID or permalink.",
		Attributes:          virtualMachinePackageSchemaAttributes(true),
	}
}

func (d *VirtualMachinePackageDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data VirtualMachinePackageDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	params := &core.GetVirtualMachinePackageParams{}
	selectorName, selectorValue := selectedStringSelector(data.ID, data.Permalink)
	if selectorName == "id" {
		params.VirtualMachinePackageId = data.ID.ValueStringPointer()
	} else {
		params.VirtualMachinePackagePermalink = data.Permalink.ValueStringPointer()
	}

	res, err := d.M.Core.GetVirtualMachinePackageWithResponse(ctx, params)
	if err != nil {
		if res != nil {
			err = genericAPIError(err, res.Body)
		}
		resp.Diagnostics.AddError("Virtual Machine Package Error", err.Error())
		return
	}
	if res == nil || res.JSON200 == nil {
		detail := fmt.Sprintf(
			"No virtual machine package with %s %q exists.",
			selectorName, selectorValue,
		)
		if res != nil {
			if apiErr := parseGenericAPIError(res.Body); apiErr != nil {
				detail = apiErr.Error()
			}
		}
		resp.Diagnostics.AddError("Virtual Machine Package Not Found", detail)
		return
	}

	data = virtualMachinePackageDataSourceModel(
		&res.JSON200.VirtualMachinePackage,
	)
	if data.ID.IsNull() {
		resp.Diagnostics.AddError(
			"Virtual Machine Package Error",
			"virtual machine package response did not include an ID",
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func virtualMachinePackageDataSourceModel(
	pkg *core.VirtualMachinePackage,
) VirtualMachinePackageDataSourceModel {
	return VirtualMachinePackageDataSourceModel{
		ID:            types.StringPointerValue(pkg.Id),
		Name:          types.StringPointerValue(pkg.Name),
		Permalink:     types.StringPointerValue(pkg.Permalink),
		CPUCores:      intPointerValue(pkg.CpuCores),
		IPv4Addresses: intPointerValue(pkg.Ipv4Addresses),
		MemoryInGB:    intPointerValue(pkg.MemoryInGb),
		StorageInGB:   intPointerValue(pkg.StorageInGb),
		Privacy:       stringerPointerValue(pkg.Privacy),
	}
}

func fetchAllVirtualMachinePackages(
	ctx context.Context,
	m *Meta,
) ([]core.VirtualMachinePackage, error) {
	packages := []core.VirtualMachinePackage{}
	for page := 1; ; page++ {
		res, err := m.Core.GetVirtualMachinePackagesWithResponse(
			ctx, &core.GetVirtualMachinePackagesParams{
				Page:    &page,
				PerPage: ptr(virtualMachinePackagesPageSize),
			},
		)
		if err != nil {
			if res != nil {
				err = genericAPIError(err, res.Body)
			}
			return nil, err
		}
		if res == nil || res.JSON200 == nil {
			if res != nil {
				if apiErr := parseGenericAPIError(res.Body); apiErr != nil {
					return nil, apiErr
				}
			}
			return nil, fmt.Errorf(
				"unexpected empty response listing virtual machine packages on page %d",
				page,
			)
		}

		pagePackages := res.JSON200.VirtualMachinePackages
		packages = append(packages, pagePackages...)
		if !paginationHasNext(
			res.JSON200.Pagination,
			page,
			len(pagePackages),
			virtualMachinePackagesPageSize,
		) {
			break
		}
	}

	return packages, nil
}
