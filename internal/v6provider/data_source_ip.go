package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/core"
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
				Optional: true,
				Computed: true,
			},
			"version": schema.Int64Attribute{
				Description: "IPv4 or IPv6.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.Int64{
					int64validator.OneOf(4, 6),
				},
			},
			"address": schema.StringAttribute{
				Computed: true,
				Optional: true,
			},
			"address_with_mask": schema.StringAttribute{
				Computed: true,
			},
			"reverse_dns": schema.StringAttribute{
				Computed: true,
			},
			"vip": schema.BoolAttribute{
				Optional: true,
				Computed: true,
			},
			"label": schema.StringAttribute{
				Description:         "VIP label.",
				MarkdownDescription: "VIP label.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.AlsoRequires(
						path.MatchRelative().AtParent().AtName("vip")),
					stringvalidator.LengthAtLeast(1),
				},
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

	var ip *core.IPAddress
	var err error

	switch {
	case data.ID.ValueString() != "":
		ip, _, err = r.M.Core.IPAddresses.GetByID(ctx, data.ID.ValueString())
	default:
		ip, _, err = r.M.Core.IPAddresses.GetByAddress(ctx,
			data.Address.ValueString())
	}

	if err != nil {
		resp.Diagnostics.AddError(
			"IP Error",
			err.Error(),
		)
		return
	}

	if ip.Network != nil {
		data.NetworkID = types.StringValue(ip.Network.ID)
	}

	data.ID = types.StringValue(ip.ID)
	data.Address = types.StringValue(ip.Address)
	data.AddressWithMask = types.StringValue(ip.AddressWithMask)
	data.ReverseDNS = types.StringValue(ip.ReverseDNS)
	data.Version = types.Int64Value(flattenIPVersion(ip.Address))
	data.VIP = types.BoolValue(ip.VIP)
	data.Label = types.StringValue(ip.Label)
	data.AllocationType = types.StringValue(ip.AllocationType)
	data.AllocationID = types.StringValue(ip.AllocationID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
