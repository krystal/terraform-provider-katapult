package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

type NetworksDataSource struct {
	M *Meta
}

type NetworksDataSourceModel struct {
	Networks []NetworkDataSourceModel `tfsdk:"networks"`
}

func (nds NetworksDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_networks"
}

func (nds *NetworksDataSource) Configure(
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

func (nds *NetworksDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"networks": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:    true,
							Description: "The ID of this resource.",
						},
						"name": schema.StringAttribute{
							Computed:    true,
							Description: "The name of the network.",
						},
						"permalink": schema.StringAttribute{
							Computed:    true,
							Description: "The permalink of the network.",
						},
						"data_center_id": schema.StringAttribute{
							Computed: true,
							Description: "The ID of the data center this " +
								"network belongs to.",
						},
						"default": schema.BoolAttribute{
							Computed: true,
							Description: "True if this is the default " +
								"network for the data center it belongs to.",
						},
					},
				},
			},
		},
	}
}

func (nds *NetworksDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data NetworksDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, err := nds.M.Core.GetOrganizationAvailableNetworksWithResponse(
		ctx,
		&core.GetOrganizationAvailableNetworksParams{
			OrganizationSubDomain: &nds.M.confOrganization,
		},
	)
	if err != nil {
		if res != nil {
			err = genericAPIError(err, res.Body)
		}

		resp.Diagnostics.AddError("Networks Error", err.Error())
		return
	}

	networks := res.JSON200.Networks
	list := make([]NetworkDataSourceModel, len(networks))

	for i, network := range networks {
		model := NetworkDataSourceModel{
			ID:      types.StringPointerValue(network.Id),
			Name:    types.StringPointerValue(network.Name),
			Default: types.BoolPointerValue(network.Default),
		}

		if v, err := network.Permalink.Get(); err == nil {
			model.Permalink = types.StringValue(v)
		}

		if network.DataCenter != nil {
			model.DataCenterID = types.StringPointerValue(network.DataCenter.Id)
		}

		list[i] = model
	}

	data.Networks = list
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
