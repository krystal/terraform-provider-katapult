package v6provider

import (
	"context"
	"errors"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult"
	"github.com/krystal/go-katapult/core"
)

type (
	IPResource struct {
		M *Meta
	}

	IPResourceModel struct {
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

func (r IPResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_ip"
}

func (r *IPResource) Configure(
	_ context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
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

func (r IPResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"network_id": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"version": schema.Int64Attribute{
				Description: "IPv4 or IPv6.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(4),
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
				Validators: []validator.Int64{
					int64validator.OneOf(4, 6),
				},
			},
			"address": schema.StringAttribute{
				Computed: true,
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
				Default:  booldefault.StaticBool(false),
			},
			"label": schema.StringAttribute{
				Description: "VIP label. Required when vip is true.",
				MarkdownDescription: "VIP label." +
					"Required when **vip** is `true`.",
				Optional: true,
				Computed: true,
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

func (r *IPResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan IPResourceModel

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var network *core.Network

	if plan.NetworkID.ValueString() != "" {
		network = &core.Network{
			ID: plan.NetworkID.ValueString(),
		}
	} else {
		var err error
		network, _, err = r.M.Core.DataCenters.DefaultNetwork(
			ctx, r.M.DataCenterRef,
		)

		if err != nil {
			resp.Diagnostics.AddError(
				"Default Network Error",
				err.Error(),
			)
			return
		}
	}

	args := &core.IPAddressCreateArguments{
		Network: network.Ref(),
		Version: unflattenIPVersion(plan.Version.ValueInt64()),
	}

	if vip := plan.VIP.ValueBool(); vip {
		args.VIP = &vip
		args.Label = plan.Label.ValueString()
	}

	ip, _, err := r.M.Core.IPAddresses.Create(ctx, r.M.OrganizationRef, args)
	if err != nil {
		resp.Diagnostics.AddError("IP Address Create Error", err.Error())
		return
	}

	plan.ID = types.StringValue(ip.ID)

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)

	if err := r.IPRead(ctx, &plan, &resp.State); err != nil {
		resp.Diagnostics.AddError("IP Address Read Error", err.Error())
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *IPResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	state := &IPResourceModel{}
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.IPRead(ctx, state, &resp.State); err != nil {
		resp.Diagnostics.AddError("IP Address Read Error", err.Error())
		return
	}

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)

	/* ... */
}

func (r *IPResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan IPResourceModel

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var state IPResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ipRef := core.IPAddressRef{ID: plan.ID.String()}
	args := &core.IPAddressUpdateArguments{}

	if !plan.VIP.Equal(state.VIP) {
		vip := plan.VIP.ValueBool()
		args.VIP = &vip
	}

	if !plan.Label.Equal(state.Label) {
		args.Label = plan.Label.ValueString()
	}

	_, _, err := r.M.Core.IPAddresses.Update(ctx, ipRef, args)
	if err != nil {
		resp.Diagnostics.AddError("IP Address Update Error", err.Error())
		return
	}

	if err := r.IPRead(ctx, &plan, &resp.State); err != nil {
		resp.Diagnostics.AddError("IP Address Read Error", err.Error())
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *IPResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	state := &IPResourceModel{}
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ipRef := core.IPAddressRef{ID: state.ID.ValueString()}
	_, err := r.M.Core.IPAddresses.Delete(ctx, ipRef)
	if err != nil {
		resp.Diagnostics.AddError("IP Address Delete Error", err.Error())
	}
}

func (r *IPResource) IPRead(
	ctx context.Context,
	model *IPResourceModel,
	state *tfsdk.State,
) error {
	ip, _, err := r.M.Core.IPAddresses.GetByID(ctx, model.ID.ValueString())
	if err != nil {
		if errors.Is(err, katapult.ErrNotFound) {
			state.RemoveResource(ctx)
			return nil
		}

		return err
	}

	if ip.Network != nil {
		model.NetworkID = types.StringValue(ip.Network.ID)
	}

	model.Address = types.StringValue(ip.Address)
	model.AddressWithMask = types.StringValue(ip.AddressWithMask)
	model.ReverseDNS = types.StringValue(ip.ReverseDNS)
	model.Version = types.Int64Value(flattenIPVersion(ip.Address))
	model.VIP = types.BoolValue(ip.VIP)
	model.Label = types.StringValue(ip.Label)
	model.AllocationType = types.StringValue(ip.AllocationType)
	model.AllocationID = types.StringValue(ip.AllocationID)

	return nil
}

func (r *IPResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func unflattenIPVersion(ver int64) core.IPVersion {
	switch ver {
	case 6:
		return core.IPv6
	default:
		return core.IPv4
	}
}

func flattenIPVersion(address string) int64 {
	if strings.Count(address, ":") < 2 {
		return 4
	}

	return 6
}
