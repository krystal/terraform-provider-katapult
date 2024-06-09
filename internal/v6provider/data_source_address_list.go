package v6provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	core "github.com/krystal/go-katapult/next/core"
)

type (
	AddressListDataSource struct {
		M *Meta
	}

	AddressListDataSourceModel struct {
		ID   types.String `tfsdk:"id"`
		Name types.String `tfsdk:"name"`
	}
)

func (ds *AddressListDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_address_list"
}

func (ds *AddressListDataSource) Configure(
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

func (ds *AddressListDataSource) Schema(
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
		},
	}
}

func (ds *AddressListDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data AddressListDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	res, err := ds.M.Core.GetAddressListWithResponse(ctx,
		&core.GetAddressListParams{
			AddressListId: data.ID.ValueStringPointer(),
		})
	if err != nil {
		resp.Diagnostics.AddError("Address List get by ID error", err.Error())

		return
	}

	if res.JSON200 == nil {
		resp.Diagnostics.AddError(
			"failed to get address list",
			fmt.Sprintf("response code was %d", res.StatusCode()),
		)

		return
	}

	addressList := res.JSON200.AddressList

	data.ID = types.StringPointerValue(addressList.Id)
	data.Name = types.StringPointerValue(addressList.Name)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
