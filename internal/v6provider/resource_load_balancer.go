package v6provider

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	core "github.com/krystal/go-katapult/next/core"
)

type (
	LoadBalancerResource struct {
		M *Meta
	}

	LoadBalancerResourceModel struct {
		ID                     types.String `tfsdk:"id"`
		Name                   types.String `tfsdk:"name"`
		VirtualMachineIDs      types.Set    `tfsdk:"virtual_machine_ids"`
		VirtualMachineGroupIDs types.Set    `tfsdk:"virtual_machine_group_ids"`
		TagIDs                 types.Set    `tfsdk:"tag_ids"`
		IPAddress              types.String `tfsdk:"ip_address"`
		HTTPSRedirect          types.Bool   `tfsdk:"https_redirect"`
	}
)

func (r *LoadBalancerResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_load_balancer"
}

func (r *LoadBalancerResource) Configure(
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

func LoadBalancerType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"id":   types.StringType,
			"name": types.StringType,
			"virtual_machine_ids": types.SetType{
				ElemType: types.StringType,
			},
			"virtual_machine_group_ids": types.SetType{
				ElemType: types.StringType,
			},
			"tag_ids": types.SetType{
				ElemType: types.StringType,
			},
			"ip_address":     types.StringType,
			"https_redirect": types.BoolType,
		},
	}
}

func (r LoadBalancerResource) Schema(
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
			"name": schema.StringAttribute{
				Required: true,
			},
			"virtual_machine_ids": schema.SetAttribute{
				Optional: true,
				Computed: true,
				Validators: []validator.Set{
					setvalidator.ConflictsWith(
						path.MatchRoot("tag_ids"),
						path.MatchRoot("virtual_machine_group_ids"),
					),
				},
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.Set{
					loadBalancerResourceIDsPlanModifier(),
				},
			},
			"virtual_machine_group_ids": schema.SetAttribute{
				Optional: true,
				Computed: true,
				Validators: []validator.Set{
					setvalidator.ConflictsWith(
						path.MatchRoot("tag_ids"),
						path.MatchRoot("virtual_machine_ids"),
					),
				},
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.Set{
					loadBalancerResourceIDsPlanModifier(),
				},
			},
			"tag_ids": schema.SetAttribute{
				Optional: true,
				Computed: true,
				Validators: []validator.Set{
					setvalidator.ConflictsWith(
						path.MatchRoot("virtual_machine_ids"),
						path.MatchRoot("virtual_machine_group_ids"),
					),
				},
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.Set{
					loadBalancerResourceIDsPlanModifier(),
				},
			},
			"ip_address": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"https_redirect": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *LoadBalancerResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan LoadBalancerResourceModel

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	name := r.M.UseOrGenerateName(plan.Name.ValueString())

	t, ids := extractLoadBalancerResourceTypeAndIDs(&plan)
	if t == "" {
		t = core.VirtualMachines
	}

	args := core.PostOrganizationLoadBalancersJSONRequestBody{
		Organization: core.OrganizationLookup{
			SubDomain: &r.M.confOrganization,
		},
		Properties: core.LoadBalancerArguments{
			Name:          &name,
			ResourceType:  &t,
			ResourceIds:   &ids,
			HttpsRedirect: plan.HTTPSRedirect.ValueBoolPointer(),
			DataCenter: &core.DataCenterLookup{
				Permalink: &r.M.confDataCenter,
			},
		},
	}

	res, err := r.M.Core.
		PostOrganizationLoadBalancersWithResponse(ctx, args)
	if err != nil {
		resp.Diagnostics.AddError("Load Balancer Create Error", err.Error())
		return
	}

	id := *res.JSON201.LoadBalancer.Id

	if err := r.LoadBalancerRead(ctx, id, &plan); err != nil {
		resp.Diagnostics.AddError("Load Balancer Read Error", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *LoadBalancerResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	state := &LoadBalancerResourceModel{}
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.LoadBalancerRead(ctx, state.ID.ValueString(), state)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			r.M.Logger.Info(
				"Load Balancer not found, removing from state",
				"id", state.ID.ValueString(),
			)
			resp.State.RemoveResource(ctx)

			return
		}

		resp.Diagnostics.AddError("Load Balancer Read Error", err.Error())
		return
	}

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *LoadBalancerResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan LoadBalancerResourceModel

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var state LoadBalancerResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()

	args := core.PatchLoadBalancerJSONRequestBody{
		LoadBalancer: core.LoadBalancerLookup{Id: &id},
		Properties:   core.LoadBalancerArguments{},
	}

	if !plan.Name.Equal(state.Name) {
		args.Properties.Name = plan.Name.ValueStringPointer()
	}

	if !plan.HTTPSRedirect.Equal(state.HTTPSRedirect) {
		args.Properties.HttpsRedirect = plan.HTTPSRedirect.ValueBoolPointer()
	}

	if !plan.VirtualMachineIDs.Equal(state.VirtualMachineIDs) ||
		!plan.VirtualMachineGroupIDs.Equal(state.VirtualMachineGroupIDs) ||
		!plan.TagIDs.Equal(state.TagIDs) {
		t, ids := extractLoadBalancerResourceTypeAndIDs(&plan)
		args.Properties.ResourceType = &t
		args.Properties.ResourceIds = &ids
	}

	_, err := r.M.Core.PatchLoadBalancerWithResponse(ctx, args)
	if err != nil {
		resp.Diagnostics.AddError("Load Balancer Update Error", err.Error())
		return
	}

	if err := r.LoadBalancerRead(ctx, id, &plan); err != nil {
		resp.Diagnostics.AddError("Load Balancer Read Error", err.Error())
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *LoadBalancerResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	state := &LoadBalancerResourceModel{}
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.M.Core.DeleteLoadBalancerWithResponse(ctx,
		core.DeleteLoadBalancerJSONRequestBody{
			LoadBalancer: core.LoadBalancerLookup{
				Id: state.ID.ValueStringPointer(),
			},
		})
	if err != nil {
		resp.Diagnostics.AddError("Load Balancer Delete Error", err.Error())
	}
}

func (r *LoadBalancerResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *LoadBalancerResource) LoadBalancerRead(
	ctx context.Context,
	id string,
	model *LoadBalancerResourceModel,
) error {
	res, err := r.M.Core.GetLoadBalancerWithResponse(
		ctx,
		&core.GetLoadBalancerParams{
			LoadBalancerId: &id,
		})
	if err != nil {
		return err
	}

	lb := res.JSON200.LoadBalancer

	model.ID = types.StringValue(id)
	model.Name = types.StringPointerValue(lb.Name)

	model.HTTPSRedirect = types.BoolPointerValue(lb.HttpsRedirect)
	if lb.IpAddress != nil {
		model.IPAddress = types.StringPointerValue(lb.IpAddress.Address)
	}

	populateLoadBalancerTargets(model, *lb.ResourceType, *lb.ResourceIds)

	return nil
}

func populateLoadBalancerTargets(
	model *LoadBalancerResourceModel,
	t core.LoadBalancerResourceTypesEnum,
	ids []string,
) {
	list := flattenLoadBalancerResourceIDs(ids)
	model.VirtualMachineIDs = types.SetValueMust(
		types.StringType, []attr.Value{},
	)
	model.VirtualMachineGroupIDs = types.SetValueMust(
		types.StringType, []attr.Value{},
	)
	model.TagIDs = types.SetValueMust(types.StringType, []attr.Value{})

	if len(ids) == 0 {
		return
	}

	switch t {
	case core.VirtualMachines:
		model.VirtualMachineIDs = list
	case core.VirtualMachineGroups:
		model.VirtualMachineGroupIDs = list
	case core.Tags:
		model.TagIDs = list
	}
}

func flattenLoadBalancerResourceIDs(ids []string) types.Set {
	values := make([]attr.Value, len(ids))

	for i, id := range ids {
		values[i] = types.StringValue(id)
	}

	return types.SetValueMust(types.StringType, values)
}

func extractLoadBalancerResourceTypeAndIDs(
	model *LoadBalancerResourceModel,
) (core.LoadBalancerResourceTypesEnum, []string) {
	var t core.LoadBalancerResourceTypesEnum = core.VirtualMachines
	var list []attr.Value
	ids := []string{}

	//nolint:lll
	switch {
	case !model.VirtualMachineIDs.IsNull() && len(model.VirtualMachineIDs.Elements()) > 0:
		t = core.VirtualMachines
		list = model.VirtualMachineIDs.Elements()
	case !model.VirtualMachineGroupIDs.IsNull() && len(model.VirtualMachineGroupIDs.Elements()) > 0:
		t = core.VirtualMachineGroups
		list = model.VirtualMachineGroupIDs.Elements()
	case !model.TagIDs.IsNull() && len(model.TagIDs.Elements()) > 0:
		t = core.Tags
		list = model.TagIDs.Elements()
	}

	for _, item := range list {
		i := item.(types.String)

		ids = append(ids, i.ValueString())
	}

	return t, ids
}

// loadBalancerResourceIDsPlanModifier handles the planning of the resource IDs
// attributes for the load balancer resource. This is needed to ensure correct
// planning when between one of the three attributes used to specify resource
// IDs of different types.
//
// It is based on setplanmanager.UseStateForUnknown(), and behaves identically
// to it when the attribute being planned is configured with one or more values
// in the config.
//
// When the attribute being planned however is not the with values, it will
// forcibly set the planned value to a empty list. Without this, Terraform does
// not realize that the old attribute needs to be cleared out when switching
// between VM IDs, VM Group IDs, and Tag IDs.
func loadBalancerResourceIDsPlanModifier() planmodifier.Set {
	return &loadBalancerResourceIDsModifier{}
}

type loadBalancerResourceIDsModifier struct{}

var _ planmodifier.Set = &loadBalancerResourceIDsModifier{}

func (m *loadBalancerResourceIDsModifier) Description(
	_ context.Context,
) string {
	return "Handles load balancer resource ID change planning."
}

func (m *loadBalancerResourceIDsModifier) MarkdownDescription(
	_ context.Context,
) string {
	return "Handles load balancer resource ID change planning."
}

func (m *loadBalancerResourceIDsModifier) PlanModifySet(
	ctx context.Context,
	req planmodifier.SetRequest,
	resp *planmodifier.SetResponse,
) {
	// Do nothing if there is no state value.
	if req.StateValue.IsNull() {
		return
	}

	// Do nothing if there is an unknown configuration value, otherwise
	// interpolation gets messed up.
	if req.ConfigValue.IsUnknown() {
		return
	}

	model := LoadBalancerResourceModel{}
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)

	// Determine the resource type based on which attribute has one or more
	// elements, and extract the path based on the `tfsdk` struct tag.
	var resourceTypeAttr string
	switch {
	case len(model.VirtualMachineIDs.Elements()) > 0:
		resourceTypeAttr = getTagValue(model, "VirtualMachineIDs", "tfsdk")
	case len(model.VirtualMachineGroupIDs.Elements()) > 0:
		resourceTypeAttr = getTagValue(model, "VirtualMachineGroupIDs", "tfsdk")
	case len(model.TagIDs.Elements()) > 0:
		resourceTypeAttr = getTagValue(model, "TagIDs", "tfsdk")
	}

	// Set the plan value to empty if:
	//
	// - None of the resource ID attributes have any values.
	// - Current path is not the one which has one or more values.
	// - Current path has value in plan, but not in the configuration,
	//   indicating it has been removed.
	if resourceTypeAttr == "" || !req.Path.Equal(path.Root(resourceTypeAttr)) ||
		len(req.ConfigValue.Elements()) == 0 {
		resp.PlanValue = types.SetValueMust(types.StringType, []attr.Value{})
		return
	}

	// Do nothing if there is a known planned value.
	if !req.PlanValue.IsUnknown() {
		return
	}

	// Set the plan value to the state value if the resource type is that of the
	// current path.
	resp.PlanValue = req.StateValue
}
