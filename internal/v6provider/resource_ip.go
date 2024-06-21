package v6provider

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	core "github.com/krystal/go-katapult/next/core"
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
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"network_id": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"version": schema.Int64Attribute{
				Description: "IPv4 or IPv6. Default is `4`.",
				MarkdownDescription: "IPv4 or IPv6. " +
					"Default is `4`.",
				Optional: true,
				Computed: true,
				Default:  int64default.StaticInt64(4),
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
				Validators: []validator.Int64{
					int64validator.OneOf(4, 6),
				},
			},
			"address": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"address_with_mask": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"reverse_dns": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"vip": schema.BoolAttribute{
				Description:         "Default is `false`.",
				MarkdownDescription: "Default is `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"label": schema.StringAttribute{
				Description: "VIP label. Required when vip is true.",
				MarkdownDescription: "VIP label. " +
					"Required when **vip** is `true`.",
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					stringvalidator.AlsoRequires(
						path.MatchRoot("vip")),
					stringvalidator.LengthAtLeast(1),
				},
			},
			"allocation_type": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"allocation_id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
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

	var networkID *string

	if netID := plan.NetworkID.ValueString(); netID != "" {
		networkID = &netID
	} else {
		res, err := r.M.Core.GetDataCenterDefaultNetworkWithResponse(ctx,
			&core.GetDataCenterDefaultNetworkParams{
				DataCenterPermalink: &r.M.confDataCenter,
			})
		if err != nil {
			resp.Diagnostics.AddError(
				"Default Network Error",
				err.Error(),
			)
			return
		}

		networkID = res.JSON200.Network.Id
	}

	args := core.PostOrganizationIpAddressesJSONRequestBody{
		Organization: core.OrganizationLookup{
			SubDomain: &r.M.confOrganization,
		},
		Network: core.NetworkLookup{
			Id: networkID,
		},
		Version: unflattenIPVersion(plan.Version.ValueInt64()),
	}

	if vip := plan.VIP.ValueBool(); vip {
		args.Vip = &vip
		args.Label = plan.Label.ValueStringPointer()
	}

	res, err := r.M.Core.PostOrganizationIpAddressesWithResponse(ctx, args)
	if err != nil {
		resp.Diagnostics.AddError("IP Address Create Error", err.Error())
		return
	}
	if res.StatusCode() < 200 || res.StatusCode() >= 300 {
		resp.Diagnostics.AddError(
			"IP Address Create Error",
			string(res.Body),
		)
		return
	}

	id := res.JSON200.IpAddress.Id

	plan.ID = types.StringPointerValue(id)

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	if err := r.IPRead(ctx, *id, &plan); err != nil {
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

	if err := r.IPRead(ctx, state.ID.ValueString(), state); err != nil {
		if errors.Is(err, ErrNotFound) {
			r.M.Logger.Info(
				"IP Address not found, removing from state",
				"id", state.ID.ValueString(),
			)
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError("IP Address Read Error", err.Error())
		return
	}

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
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
	id := state.ID.ValueString()

	args := core.PatchIpAddressJSONRequestBody{
		IpAddress: core.IPAddressLookup{
			Id: &id,
		},
	}

	if !plan.VIP.Equal(state.VIP) {
		args.Vip = plan.VIP.ValueBoolPointer()
	}

	if !plan.Label.Equal(state.Label) {
		args.Label = plan.Label.ValueStringPointer()
	}

	_, err := r.M.Core.PatchIpAddressWithResponse(ctx, args)
	if err != nil {
		resp.Diagnostics.AddError("IP Address Update Error", err.Error())
		return
	}

	if err := r.IPRead(ctx, id, &plan); err != nil {
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

	_, err := r.M.Core.DeleteIpAddressWithResponse(ctx,
		core.DeleteIpAddressJSONRequestBody{
			IpAddress: core.IPAddressLookup{
				Id: state.ID.ValueStringPointer(),
			},
		})
	if err != nil {
		resp.Diagnostics.AddError("IP Address Delete Error", err.Error())
	}
}

func (r *IPResource) IPRead(
	ctx context.Context,
	id string,
	model *IPResourceModel,
) error {
	res, err := r.M.Core.GetIpAddressWithResponse(ctx,
		&core.GetIpAddressParams{
			IpAddressId: &id,
		},
	)
	if res.StatusCode() == http.StatusNotFound {
		return ErrNotFound
	}

	if err != nil {
		return err
	}

	ip := res.JSON200.IpAddress

	if ip.Network != nil {
		model.NetworkID = types.StringPointerValue(ip.Network.Id)
	}

	model.Address = types.StringPointerValue(ip.Address)
	model.AddressWithMask = types.StringPointerValue(ip.AddressWithMask)
	model.ReverseDNS = types.StringPointerValue(ip.ReverseDns)
	if ip.Address != nil {
		model.Version = types.Int64Value(flattenIPVersion(*ip.Address))
	}
	model.VIP = types.BoolPointerValue(ip.Vip)

	if ip.Label != nil {
		model.Label = types.StringPointerValue(ip.Label)
	} else {
		model.Label = types.StringValue("")
	}

	if ip.AllocationType != nil {
		model.AllocationType = types.StringPointerValue(ip.AllocationType)
	} else {
		model.AllocationType = types.StringValue("")
	}

	if ip.AllocationId != nil {
		model.AllocationID = types.StringPointerValue(ip.AllocationId)
	} else {
		model.AllocationID = types.StringValue("")
	}

	return nil
}

func (r *IPResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func unflattenIPVersion(ver int64) core.IPAddressVersionEnum {
	switch ver {
	case 6:
		return core.Ipv6
	default:
		return core.Ipv4
	}
}

func flattenIPVersion(address string) int64 {
	if strings.Count(address, ":") < 2 {
		return 4
	}

	return 6
}
