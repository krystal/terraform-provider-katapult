package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	core "github.com/krystal/go-katapult/next/core"
)

type (
	DataCenterDataSource struct {
		M *Meta
	}

	DataCenterDataSourceModel struct {
		ID          types.String `tfsdk:"id"`
		Name        types.String `tfsdk:"name"`
		Permalink   types.String `tfsdk:"permalink"`
		CountryID   types.String `tfsdk:"country_id"`
		CountryName types.String `tfsdk:"country_name"`
	}
)

func (d *DataCenterDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_data_center"
}

func (d *DataCenterDataSource) Configure(
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

func (d *DataCenterDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieves a data center by ID or permalink. " +
			"When neither is configured, the provider's data center is returned.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The ID of this resource.",
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The name of the data center.",
			},
			"permalink": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The permalink of the data center.",
			},
			"country_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ID of the data center's country.",
			},
			"country_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The name of the data center's country.",
			},
		},
	}
}

func (d *DataCenterDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data DataCenterDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	params := &core.GetDataCenterParams{}
	switch {
	case !data.ID.IsNull() && !data.ID.IsUnknown() && data.ID.ValueString() != "":
		params.DataCenterId = data.ID.ValueStringPointer()
	case !data.Permalink.IsNull() && !data.Permalink.IsUnknown() &&
		data.Permalink.ValueString() != "":
		params.DataCenterPermalink = data.Permalink.ValueStringPointer()
	default:
		params.DataCenterPermalink = &d.M.confDataCenter
	}

	res, err := d.M.Core.GetDataCenterWithResponse(ctx, params)
	if err != nil {
		if res != nil {
			err = genericAPIError(err, res.Body)
		}
		resp.Diagnostics.AddError("Data Center Error", err.Error())
		return
	}
	if res == nil || res.JSON200 == nil {
		detail := "unexpected empty response fetching data center"
		if res != nil {
			if apiErr := parseGenericAPIError(res.Body); apiErr != nil {
				detail = apiErr.Error()
			}
		}
		resp.Diagnostics.AddError("Data Center Error", detail)
		return
	}

	data = dataCenterDataSourceModel(&res.JSON200.DataCenter)
	if data.ID.IsNull() {
		resp.Diagnostics.AddError(
			"Data Center Error",
			"data center response did not include an ID",
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func dataCenterDataSourceModel(
	dataCenter *core.GetDataCenter200ResponseDataCenter,
) DataCenterDataSourceModel {
	model := DataCenterDataSourceModel{
		ID:          types.StringPointerValue(dataCenter.Id),
		Name:        types.StringPointerValue(dataCenter.Name),
		Permalink:   types.StringPointerValue(dataCenter.Permalink),
		CountryID:   types.StringNull(),
		CountryName: types.StringNull(),
	}
	if dataCenter.Country != nil {
		model.CountryID = types.StringPointerValue(dataCenter.Country.Id)
		model.CountryName = types.StringPointerValue(dataCenter.Country.Name)
	}

	return model
}
