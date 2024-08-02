package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	core "github.com/krystal/go-katapult/next/core"
)

type (
	AddressListEntryDataSource struct {
		M *Meta
	}

	AddressListEntryDataSourceModel struct {
		ID      types.String `tfsdk:"id"`
		Name    types.String `tfsdk:"name"`
		Address types.String `tfsdk:"address"`
	}
)

func (ds *AddressListEntryDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_address_list_entry"
}

func (ds *AddressListEntryDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}

	m, ok := req.ProviderData.(*Meta)
	if !ok {
		resp.Diagnostics.AddError(
			"Meta Error",
			"meta is not of type *Meta",
		)
		return
	}

	ds.M = m
}

func (ds *AddressListEntryDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required: true,
			},
			"name": schema.StringAttribute{
				Computed: true,
			},
			"address": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

func (ds *AddressListEntryDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data AddressListEntryDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, err := ds.M.Core.GetAddressListEntryWithResponse(ctx,
		&core.GetAddressListEntryParams{
			AddressListEntryId: data.ID.ValueStringPointer(),
		})
	if err != nil {
		resp.Diagnostics.AddError(
			"Address List Entry get by ID error",
			err.Error())

		return
	}

	entry := res.JSON200.AddressListEntry

	data.ID = types.StringPointerValue(entry.Id)
	data.Name = types.StringPointerValue(entry.Name)
	data.Address = types.StringPointerValue(entry.Address)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
