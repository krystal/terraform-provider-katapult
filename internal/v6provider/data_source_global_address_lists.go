package v6provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	core "github.com/krystal/go-katapult/next/core"
)

type (
	GlobalAddressListsDataSource struct {
		M *Meta
	}

	GlobalAddressListsDataSourceModel struct {
		AddressLists types.Set `tfsdk:"address_lists"`
	}
)

func (ds *GlobalAddressListsDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_global_address_lists"
}

func (ds *GlobalAddressListsDataSource) Configure(
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

func (ds *GlobalAddressListsDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"address_lists": schema.SetNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed: true,
						},
						"name": schema.StringAttribute{
							Computed: true,
						},
					},
				},
			},
		},
	}
}

func (ds *GlobalAddressListsDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data GlobalAddressListsDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	addressLists := []core.GetAddressLists200ResponseAddressLists{}
	totalPages := 2

	for i := 1; i < totalPages; i++ {
		res, err := ds.M.Core.GetAddressListsWithResponse(ctx,
			&core.GetAddressListsParams{
				Page: &i,
			})
		if err != nil {
			resp.Diagnostics.AddError("Address Lists get error", err.Error())

			return
		}

		if res.JSON200 == nil {
			resp.Diagnostics.AddError(
				"failed to get address lists",
				fmt.Sprintf("response code was %d", res.StatusCode()),
			)

			return
		}

		addressLists = append(addressLists, res.JSON200.AddressLists...)
		totalPages = *res.JSON200.Pagination.TotalPages
	}

	listValueType := types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"id":   types.StringType,
			"name": types.StringType,
		},
	}

	addrListValues, diags := convertGlobalAddrListsToValues(
		addressLists,
		listValueType.AttrTypes,
	)
	if diags != nil {
		resp.Diagnostics.Append(diags...)
		return
	}

	addrListValue, diags := types.SetValue(
		listValueType,
		addrListValues,
	)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.AddressLists = addrListValue

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func convertGlobalAddrListsToValues(
	lists []core.GetAddressLists200ResponseAddressLists,
	attrTypes map[string]attr.Type,
) ([]attr.Value, diag.Diagnostics) {
	vals := make([]attr.Value, len(lists))

	for index, list := range lists {
		listval, diags := types.ObjectValue(
			attrTypes,
			map[string]attr.Value{
				"id":   types.StringPointerValue(list.Id),
				"name": types.StringPointerValue(list.Name),
			},
		)

		if diags.HasError() {
			return nil, diags
		}

		vals[index] = listval
	}

	return vals, nil
}
