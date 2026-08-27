package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type (
	VirtualMachinePackagesDataSource struct {
		M *Meta
	}

	VirtualMachinePackagesDataSourceModel struct {
		ID       types.String                           `tfsdk:"id"`
		Packages []VirtualMachinePackageDataSourceModel `tfsdk:"packages"`
	}
)

func (d *VirtualMachinePackagesDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_virtual_machine_packages"
}

func (d *VirtualMachinePackagesDataSource) Configure(
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

func (d *VirtualMachinePackagesDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists all available virtual machine packages.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Always set to `all`.",
			},
			"packages": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "The available virtual machine packages.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: virtualMachinePackageSchemaAttributes(false),
				},
			},
		},
	}
}

func (d *VirtualMachinePackagesDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data VirtualMachinePackagesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	packages, err := fetchAllVirtualMachinePackages(ctx, d.M)
	if err != nil {
		resp.Diagnostics.AddError("Virtual Machine Packages Error", err.Error())
		return
	}

	data.ID = types.StringValue("all")
	data.Packages = make([]VirtualMachinePackageDataSourceModel, len(packages))
	for i := range packages {
		data.Packages[i] = virtualMachinePackageDataSourceModel(&packages[i])
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
