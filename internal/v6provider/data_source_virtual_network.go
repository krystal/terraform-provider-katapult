package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

type VirtualNetworkDataSource struct {
	M *Meta
}

type VirtualNetworkDataSourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	DataCenterID types.String `tfsdk:"data_center_id"`
}

func (ds VirtualNetworkDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_virtual_network"
}

func (ds *VirtualNetworkDataSource) Configure(
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

	ds.M = meta
}

func VirtualNetworkType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"id":             types.StringType,
			"name":           types.StringType,
			"data_center_id": types.StringType,
		},
	}
}

func virtualNetworkDataSourceSchemaAttrs() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Required:    true,
			Description: "The ID of this resource.",
			Validators: []validator.String{
				stringValidatorNotEmpty(),
			},
		},
		"name": schema.StringAttribute{
			Computed:    true,
			Description: "The name of the virtual network.",
		},
		"data_center_id": schema.StringAttribute{
			Computed: true,
			Description: "The ID of the data center this virtual network " +
				"belongs to.",
		},
	}
}

func (ds *VirtualNetworkDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Attributes: virtualNetworkDataSourceSchemaAttrs(),
	}
}

func (ds *VirtualNetworkDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data VirtualNetworkDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := data.ID.ValueString()

	res, err := ds.M.Core.GetVirtualNetworkWithResponse(
		ctx, &core.GetVirtualNetworkParams{VirtualNetworkId: &id},
	)
	if err != nil {
		resp.Diagnostics.AddError("Error reading virtual network", err.Error())
		return
	}

	virtualNetwork := res.JSON200.VirtualNetwork
	data.ID = types.StringPointerValue(virtualNetwork.Id)
	data.Name = types.StringPointerValue(virtualNetwork.Name)
	data.DataCenterID = types.StringPointerValue(virtualNetwork.DataCenter.Id)

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}
