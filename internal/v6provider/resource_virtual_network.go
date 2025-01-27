package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

type VirtualNetworkResource struct {
	M *Meta
}

type VirtualNetworkResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	DataCenterID types.String `tfsdk:"data_center_id"`
}

func (r *VirtualNetworkResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_virtual_network"
}

func (r *VirtualNetworkResource) Configure(
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

func (r *VirtualNetworkResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The ID of this resource.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the virtual network.",
				Validators: []validator.String{
					stringValidatorNotEmpty(),
				},
			},
			"data_center_id": schema.StringAttribute{
				Computed: true,
				Optional: true,
				Description: "The ID of the data center to create the " +
					"virtual network in.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *VirtualNetworkResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan VirtualNetworkResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	dcLookup := core.DataCenterLookup{}

	dcID := plan.DataCenterID.ValueString()
	if dcID != "" {
		dcLookup.Id = &dcID
	} else {
		dcLookup.Permalink = &r.M.confDataCenter
	}

	params := core.PostOrganizationVirtualNetworksJSONRequestBody{
		Organization: core.OrganizationLookup{
			SubDomain: &r.M.confOrganization,
		},
		DataCenter: dcLookup,
		Properties: core.VirtualNetworkArguments{
			Name: plan.Name.ValueString(),
		},
	}

	res, err := r.M.Core.PostOrganizationVirtualNetworksWithResponse(ctx,
		params,
	)
	if err != nil {
		resp.Diagnostics.AddError("Error creating virtual network", err.Error())
		return
	}

	// Create endpoint return 200 on success rather than 201.
	if res.JSON200 == nil || res.JSON200.VirtualNetwork.Id == nil {
		resp.Diagnostics.AddError(
			"Error creating virtual network", "ID not returned",
		)
		return
	}

	assignVirtualNetworkFields(&res.JSON200.VirtualNetwork, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *VirtualNetworkResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state VirtualNetworkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()

	res, err := r.M.Core.GetVirtualNetworkWithResponse(
		ctx, &core.GetVirtualNetworkParams{VirtualNetworkId: &id},
	)
	if err != nil {
		if res.JSON404 != nil {
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError("Error reading virtual network", err.Error())
		return
	}

	assignVirtualNetworkFields(&res.JSON200.VirtualNetwork, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *VirtualNetworkResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan VirtualNetworkResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	params := core.PatchVirtualNetworkJSONRequestBody{
		VirtualNetwork: core.VirtualNetworkLookup{
			Id: plan.ID.ValueStringPointer(),
		},
		Properties: core.VirtualNetworkArguments{
			Name: plan.Name.ValueString(),
		},
	}

	res, err := r.M.Core.PatchVirtualNetworkWithResponse(ctx, params)
	if err != nil {
		resp.Diagnostics.AddError("Error updating virtual network", err.Error())
		return
	}

	assignVirtualNetworkFields(&res.JSON200.VirtualNetwork, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *VirtualNetworkResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state VirtualNetworkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.M.Core.DeleteVirtualNetworkWithResponse(
		ctx, core.DeleteVirtualNetworkJSONRequestBody{
			VirtualNetwork: core.VirtualNetworkLookup{
				Id: state.ID.ValueStringPointer(),
			},
		},
	)
	if err != nil {
		resp.Diagnostics.AddError("Error deleting virtual network", err.Error())
		return
	}
}

func (r *VirtualNetworkResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func assignVirtualNetworkFields(
	network *core.VirtualNetwork,
	model *VirtualNetworkResourceModel,
) {
	model.ID = types.StringPointerValue(network.Id)
	model.Name = types.StringPointerValue(network.Name)
	model.DataCenterID = types.StringPointerValue(network.DataCenter.Id)
}
