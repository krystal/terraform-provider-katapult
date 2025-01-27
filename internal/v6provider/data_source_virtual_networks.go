package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

type VirtualNetworksDataSource struct {
	M *Meta
}

type VirtualNetworksDataSourceModel struct {
	VirtualNetworks []VirtualNetworkDataSourceModel `tfsdk:"virtual_networks"`
}

func (nds VirtualNetworksDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_virtual_networks"
}

func (nds *VirtualNetworksDataSource) Configure(
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

	nds.M = meta
}

func (nds *VirtualNetworksDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"virtual_networks": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:    true,
							Description: "The ID of this resource.",
						},
						"name": schema.StringAttribute{
							Computed:    true,
							Description: "The name of the virtual network.",
						},
						"data_center_id": schema.StringAttribute{
							Computed: true,
							Description: "The ID of the data center this " +
								"virtual network belongs to.",
						},
					},
				},
			},
		},
	}
}

func (nds *VirtualNetworksDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data VirtualNetworksDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, err := nds.M.Core.GetOrganizationVirtualNetworksWithResponse(
		ctx,
		&core.GetOrganizationVirtualNetworksParams{
			OrganizationSubDomain: &nds.M.confOrganization,
		},
	)
	if err != nil {
		resp.Diagnostics.AddError("VirtualNetworks get error", err.Error())
	}

	virtualNetworks := res.JSON200.VirtualNetworks
	list := make([]VirtualNetworkDataSourceModel, len(virtualNetworks))

	for i, virtualNetwork := range virtualNetworks {
		model := VirtualNetworkDataSourceModel{
			ID:   types.StringPointerValue(virtualNetwork.Id),
			Name: types.StringPointerValue(virtualNetwork.Name),
		}

		if virtualNetwork.DataCenter != nil {
			model.DataCenterID = types.StringPointerValue(
				virtualNetwork.DataCenter.Id,
			)
		}

		list[i] = model
	}

	data.VirtualNetworks = list
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
