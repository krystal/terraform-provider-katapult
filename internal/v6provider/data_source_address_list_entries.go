package v6provider

import (
	"context"
	"fmt"

	"github.com/davecgh/go-spew/spew"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	core "github.com/krystal/go-katapult/next/core"
)

type (
	AddressListEntriesDataSource struct {
		M *Meta
	}

	AddressListEntriesDataSourceModel struct {
		AddressListID types.String `tfsdk:"address_list_id"`
		Entries       types.Set    `tfsdk:"entries"`
	}
)

func (ds *AddressListEntriesDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_address_list_entries"
}

func (ds *AddressListEntriesDataSource) Configure(
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

func (ds *AddressListEntriesDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"address_list_id": schema.StringAttribute{
				Required: true,
			},
			"entries": schema.SetNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed: true,
						},
						"name": schema.StringAttribute{
							Computed: true,
						},
						"address": schema.StringAttribute{
							Computed: true,
						},
					},
				},
			},
		},
	}
}

func (ds *AddressListEntriesDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data AddressListEntriesDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	entries := []core.AddressListEntry{}
	totalPages := 2

	for i := 0; i < totalPages; i++ {
		res, err := ds.M.Core.GetAddressListEntriesWithResponse(ctx,
			&core.GetAddressListEntriesParams{
				AddressListId: data.AddressListID.ValueStringPointer(),
			})
		if err != nil {
			resp.Diagnostics.AddError(
				"Address List Entries get by ID error",
				err.Error())

			return
		}

		if res.JSON200 == nil {
			resp.Diagnostics.AddError(
				"failed to get address list entries",
				fmt.Sprintf("response code was %d", res.StatusCode()),
			)

			return
		}

		entries = append(entries, res.JSON200.AddressListEntries...)
		totalPages = *res.JSON200.Pagination.TotalPages
	}

	spew.Dump(entries)

	listValueType := types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"id":      types.StringType,
			"name":    types.StringType,
			"address": types.StringType,
		},
	}

	entryListValues, diags := convertAddrListEntriesToValues(
		entries,
		listValueType.AttrTypes,
	)
	if diags != nil {
		resp.Diagnostics.Append(diags...)
		return
	}

	entriesValue, diags := types.SetValue(
		listValueType,
		entryListValues,
	)

	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.Entries = entriesValue

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func convertAddrListEntriesToValues(
	entries []core.AddressListEntry,
	attrTypes map[string]attr.Type,
) ([]attr.Value, diag.Diagnostics) {
	vals := make([]attr.Value, len(entries))

	for index, entry := range entries {
		entryval, diags := types.ObjectValue(
			attrTypes,
			map[string]attr.Value{
				"id":      types.StringPointerValue(entry.Id),
				"name":    types.StringPointerValue(entry.Name),
				"address": types.StringPointerValue(entry.Address),
			},
		)
		if diags.HasError() {
			return nil, diags
		}

		vals[index] = entryval
	}

	return vals, nil
}
