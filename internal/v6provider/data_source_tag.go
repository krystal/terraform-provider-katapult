package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

type (
	TagDataSource struct {
		M *Meta
	}

	TagDataSourceModel struct {
		ID    types.String `tfsdk:"id"`
		Name  types.String `tfsdk:"name"`
		Color types.String `tfsdk:"color"`
	}
)

func (r TagDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_tag"
}

func (r *TagDataSource) Configure(
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

	r.M = meta
}

func (r TagDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier for the tag.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the tag.",
				Computed:    true,
			},

			"color": schema.StringAttribute{
				Description: "The color of the tag.",
				Computed:    true,
			},
		},
	}
}

func (r TagDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data TagDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, err := r.M.Core.GetTagWithResponse(ctx, &core.GetTagParams{
		TagId: data.ID.ValueStringPointer(),
	})
	if err != nil {
		if res != nil {
			err = genericAPIError(err, res.Body)
		}

		resp.Diagnostics.AddError("Tag Error", err.Error())
		return
	}

	tag := res.JSON200.Tag
	data.ID = types.StringPointerValue(tag.Id)
	data.Name = types.StringPointerValue(tag.Name)
	data.Color = types.StringValue(string(*tag.Color))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
