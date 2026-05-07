package v6provider

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

type (
	VirtualMachineGroupDataSource struct {
		M *Meta
	}

	VirtualMachineGroupDataSourceModel struct {
		ID        types.String `tfsdk:"id"`
		Name      types.String `tfsdk:"name"`
		Segregate types.Bool   `tfsdk:"segregate"`
	}
)

func (d *VirtualMachineGroupDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_virtual_machine_group"
}

func (d *VirtualMachineGroupDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}

	meta, ok := req.ProviderData.(*Meta)
	if !ok {
		resp.Diagnostics.AddError(
			"Meta Error",
			"meta is not of type *Meta",
		)
		return
	}

	d.M = meta
}

func (d *VirtualMachineGroupDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieve details of an existing " +
			"Virtual Machine Group by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The unique identifier of the " +
					"Virtual Machine Group to retrieve.",
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The name of the Virtual Machine Group.",
			},
			"segregate": schema.BoolAttribute{
				Computed: true,
				MarkdownDescription: "Whether Virtual Machines in this " +
					"group are segregated across separate host machines.",
			},
		},
	}
}

func (d *VirtualMachineGroupDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data VirtualMachineGroupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, err := d.M.Core.GetVirtualMachineGroupWithResponse(ctx,
		&core.GetVirtualMachineGroupParams{
			VirtualMachineGroupId: data.ID.ValueStringPointer(),
		})
	if err != nil {
		if res != nil {
			err = genericAPIError(err, res.Body)
		}
		if errors.Is(err, core.ErrNotFound) {
			resp.Diagnostics.AddError(
				"Virtual Machine Group Not Found",
				err.Error(),
			)
			return
		}
		resp.Diagnostics.AddError("Read Error", err.Error())
		return
	}

	vmg := res.JSON200.VirtualMachineGroup
	data.ID = types.StringPointerValue(vmg.Id)
	data.Name = types.StringPointerValue(vmg.Name)
	data.Segregate = types.BoolPointerValue(vmg.Segregate)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
