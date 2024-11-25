package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

type NetworkDataSource struct {
	M *Meta
}

type NetworkDataSourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Permalink    types.String `tfsdk:"permalink"`
	DataCenterID types.String `tfsdk:"data_center_id"`
	Default      types.Bool   `tfsdk:"default"`
}

func (nds NetworkDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_network"
}

func (nds *NetworkDataSource) Configure(
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

func NetworkType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"id":             types.StringType,
			"name":           types.StringType,
			"permalink":      types.StringType,
			"data_center_id": types.StringType,
			"default":        types.BoolType,
		},
	}
}

func NetworkListType() types.ObjectType {
	t := NetworkType()

	// When fetching a list of Networks we do no include the "default"
	// attribute, as it currently requires separate API calls for each data
	// center.
	delete(t.AttrTypes, "default")

	return t
}

func networkDataSourceSchemaAttrs() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:    true,
			Optional:    true,
			Description: "The ID of this resource.",
			Validators: []validator.String{
				stringvalidator.ConflictsWith(
					path.MatchRoot("permalink"),
					path.MatchRoot("data_center_id"),
				),
			},
		},
		"name": schema.StringAttribute{
			Computed:    true,
			Description: "The name of the network.",
		},
		"permalink": schema.StringAttribute{
			Computed:    true,
			Optional:    true,
			Description: "The permalink of the network.",
			Validators: []validator.String{
				stringvalidator.ConflictsWith(
					path.MatchRoot("id"),
					path.MatchRoot("data_center_id"),
				),
			},
		},
		"data_center_id": schema.StringAttribute{
			Computed: true,
			Optional: true,
			Description: "The ID of the data center this network " +
				"belongs to.",
			Validators: []validator.String{
				stringvalidator.ConflictsWith(
					path.MatchRoot("id"),
					path.MatchRoot("permalink"),
				),
			},
		},
		"default": schema.BoolAttribute{
			Computed: true,
			Description: "True if this is the default network for " +
				"the data center it belongs to.",
		},
	}
}

func (nds *NetworkDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Attributes: networkDataSourceSchemaAttrs(),
	}
}

func (nds *NetworkDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data NetworkDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Fetch default network for data center if neither ID or Permalink have
	// been specified.
	if data.ID.IsNull() && data.Permalink.IsNull() {
		params := &core.GetDataCenterDefaultNetworkParams{}

		dcID := data.DataCenterID.ValueString()
		if dcID != "" {
			params.DataCenterId = &dcID
		} else {
			params.DataCenterPermalink = &nds.M.confDataCenter
		}

		network, err := nds.getDefaultNetwork(ctx, params)
		if err != nil {
			resp.Diagnostics.AddError("Default network ID error", err.Error())
			return
		}

		nds.populate(ctx, resp, network, true)
		return
	}

	params := &core.GetNetworkParams{}

	// Lookup the Network by ID or Permalink.
	var getField string
	if !data.ID.IsNull() {
		params.NetworkId = data.ID.ValueStringPointer()
		getField = "id"
	} else if !data.Permalink.IsNull() {
		params.NetworkPermalink = data.Permalink.ValueStringPointer()
		getField = "permalink"
	}

	res, err := nds.M.Core.GetNetworkWithResponse(ctx, params)
	if err != nil {
		resp.Diagnostics.AddError(
			"Network get by "+getField+" error", err.Error(),
		)
		return
	}

	network := res.JSON200.Network
	nds.populate(ctx, resp, &network, false)
}

func (nds *NetworkDataSource) populate(
	ctx context.Context,
	resp *datasource.ReadResponse,
	network *core.Network,
	usedDefault bool,
) {
	var data NetworkDataSourceModel
	data.ID = types.StringPointerValue(network.Id)
	data.Name = types.StringPointerValue(network.Name)

	if network.Permalink.IsSpecified() && !network.Permalink.IsNull() {
		permalink, err := network.Permalink.Get()
		if err != nil {
			resp.Diagnostics.AddError("Network permalink error", err.Error())
			return
		}
		data.Permalink = types.StringValue(permalink)
	}

	if network.DataCenter == nil || network.DataCenter.Id == nil {
		// This should never happen, but lets avoid a panic if it does.
		resp.Diagnostics.AddError("returned data center ID is nil", "")
		return
	}
	data.DataCenterID = types.StringPointerValue(network.DataCenter.Id)

	// TODO: Remove this once the API returns a `default` attribute for
	// networks.
	if usedDefault {
		// We know for sure its a default network.
		data.Default = types.BoolValue(true)
	} else {
		// We need to check if this network is default or not.
		defaultNetwork, err := nds.getDefaultNetwork(ctx,
			&core.GetDataCenterDefaultNetworkParams{
				DataCenterId: network.DataCenter.Id,
			},
		)
		if err != nil {
			resp.Diagnostics.AddError("Default network ID error", err.Error())
			return
		}
		data.Default = types.BoolValue(*defaultNetwork.Id == *network.Id)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (nds *NetworkDataSource) getDefaultNetwork(
	ctx context.Context,
	params *core.GetDataCenterDefaultNetworkParams,
) (*core.Network, error) {
	res, err := nds.M.Core.GetDataCenterDefaultNetworkWithResponse(ctx, params)
	if err != nil {
		return nil, err
	}

	network := res.JSON200.Network

	return &core.Network{
		Id:        network.Id,
		Name:      network.Name,
		Permalink: network.Permalink,
		DataCenter: &core.DataCenter{
			Id:        network.DataCenter.Id,
			Name:      network.DataCenter.Name,
			Permalink: network.DataCenter.Permalink,
		},
	}, nil
}