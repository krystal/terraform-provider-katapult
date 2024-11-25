package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

type NetworksDataSource struct {
	M *Meta
}

type NetworksDataSourceModel struct {
	Networks types.List `tfsdk:"networks"`
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

func NetworkListType() types.ObjectType {
	t := NetworkType()

	// When fetching a list of Networks we do not include the "default"
	// attribute, as it currently requires separate API calls for each data
	// center.
	delete(t.AttrTypes, "default")

	return t
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
		resp.Diagnostics.AddError("Networks get error", err.Error())
	}

	networks := res.JSON200.Networks
	list := make([]attr.Value, len(networks))

	for i, network := range networks {
		permalink, err := network.Permalink.Get()
		if err != nil {
			resp.Diagnostics.AddError("Network permalink error", err.Error())
			return
		}

		attrs := map[string]attr.Value{
			"id":             types.StringPointerValue(network.Id),
			"name":           types.StringPointerValue(network.Name),
			"permalink":      types.StringValue(permalink),
			"data_center_id": types.StringPointerValue(network.DataCenter.Id),
		}

		list[i] = types.ObjectValueMust(NetworkListType().AttrTypes, attrs)
	}

	data.Networks = types.ListValueMust(
		NetworkListType(),
		list,
	)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
