package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	core "github.com/krystal/go-katapult/next/core"
)

type (
	IPDataSource struct {
		M *Meta
	}

	IPDataSourceModel struct {
		ID              types.String `tfsdk:"id"`
		NetworkID       types.String `tfsdk:"network_id"`
		Version         types.Int64  `tfsdk:"version"`
		Address         types.String `tfsdk:"address"`
		AddressWithMask types.String `tfsdk:"address_with_mask"`
		ReverseDNS      types.String `tfsdk:"reverse_dns"`
		VIP             types.Bool   `tfsdk:"vip"`
		Label           types.String `tfsdk:"label"`
		AllocationType  types.String `tfsdk:"allocation_type"`
		AllocationID    types.String `tfsdk:"allocation_id"`
	}
)

func (r IPDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_ip"
}

func (r *IPDataSource) Configure(
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

func (r IPDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Optional:    true,
				Description: "The ID of this resource.",
				Validators: []validator.String{
					stringvalidator.AtLeastOneOf(
						path.MatchRoot("id"),
						path.MatchRoot("address"),
					),
				},
			},
			"network_id": schema.StringAttribute{
				Computed: true,
			},
			"version": schema.Int64Attribute{
				Description: "IPv4 or IPv6.",
				Computed:    true,
			},
			"address": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"address_with_mask": schema.StringAttribute{
				Computed: true,
			},
			"reverse_dns": schema.StringAttribute{
				Computed: true,
			},
			"vip": schema.BoolAttribute{
				Computed: true,
			},
			"label": schema.StringAttribute{
				Description:         "VIP label.",
				MarkdownDescription: "VIP label.",
				Computed:            true,
			},
			"allocation_type": schema.StringAttribute{
				Computed: true,
			},
			"allocation_id": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

func (r *IPDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var data IPDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	var (
		res *core.GetIpAddressResponse
		err error
	)

	if !data.ID.IsNull() {
		res, err = r.M.Core.GetIpAddressWithResponse(ctx,
			&core.GetIpAddressParams{
				IpAddressId: data.ID.ValueStringPointer(),
			},
		)
	} else if !data.Address.IsNull() {
		res, err = r.M.Core.GetIpAddressWithResponse(ctx,
			&core.GetIpAddressParams{
				IpAddressAddress: data.Address.ValueStringPointer(),
			},
		)
	}

	if err != nil {
		resp.Diagnostics.AddError(
			"IP Error",
			err.Error(),
		)
		return
	}

	ip := res.JSON200.IpAddress

	if ip.Network != nil {
		data.NetworkID = types.StringPointerValue(ip.Network.Id)
	}

	data.ID = types.StringPointerValue(ip.Id)
	data.Address = types.StringPointerValue(ip.Address)
	data.AddressWithMask = types.StringPointerValue(ip.AddressWithMask)
	data.ReverseDNS = types.StringPointerValue(ip.ReverseDns)
	if ip.Address != nil {
		data.Version = types.Int64Value(flattenIPVersion(*ip.Address))
	}
	data.VIP = types.BoolPointerValue(ip.Vip)
	label, _ := ip.Label.Get()
	data.Label = types.StringValue(label)

	allocationType, _ := ip.AllocationType.Get()
	data.AllocationType = types.StringValue(allocationType)

	allocationID, _ := ip.AllocationId.Get()
	data.AllocationID = types.StringValue(allocationID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
