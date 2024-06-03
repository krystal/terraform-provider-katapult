package v6provider

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult"
	"github.com/krystal/go-katapult/core"
)

type (
	LoadBalancerResource struct {
		M *Meta
	}

	LoadBalancerResourceModel struct {
		ID                     types.String `tfsdk:"id"`
		Name                   types.String `tfsdk:"name"`
		ResourceType           types.String `tfsdk:"resource_type"`
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
			"id":            types.StringType,
			"name":          types.StringType,
			"resource_type": types.StringType,
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
	lbrSchema := LoadBalancerRuleSchemaAttributes()
	delete(lbrSchema, "load_balancer_id")
	lbrSchema["load_balancer_id"] = schema.StringAttribute{
		Optional: true,
	}

	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"name": schema.StringAttribute{
				Computed: true,
				Optional: true,
			},
			"resource_type": schema.StringAttribute{
				Computed: true,
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
			},
			"ip_address": schema.StringAttribute{
				Computed: true,
			},
			"https_redirect": schema.BoolAttribute{
				Computed: true,
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
		t = core.VirtualMachinesResourceType
	}

	args := &core.LoadBalancerCreateArguments{
		Name:         name,
		ResourceType: t,
		ResourceIDs:  &ids,
		DataCenter:   r.M.DataCenterRef,
	}

	lb, _, err := r.M.Core.LoadBalancers.Create(
		ctx, r.M.OrganizationRef, args,
	)
	if err != nil {
		resp.Diagnostics.AddError("Load Balancer Create Error", err.Error())
		return
	}

	if err := r.LoadBalancerRead(ctx, lb.ID, &plan, &resp.State); err != nil {
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

	if err := r.LoadBalancerRead(
		ctx,
		state.ID.ValueString(),
		state,
		&resp.State,
	); err != nil {
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

	lbRef := core.LoadBalancerRef{ID: id}
	args := &core.LoadBalancerUpdateArguments{}

	if !plan.Name.Equal(state.Name) {
		args.Name = plan.Name.ValueString()
	}

	if !plan.VirtualMachineIDs.Equal(state.VirtualMachineIDs) ||
		!plan.VirtualMachineGroupIDs.Equal(state.VirtualMachineGroupIDs) ||
		!plan.TagIDs.Equal(state.TagIDs) {
		t, ids := extractLoadBalancerResourceTypeAndIDs(&plan)
		args.ResourceType = t
		args.ResourceIDs = &ids
	}

	_, _, err := r.M.Core.LoadBalancers.Update(ctx, lbRef, args)
	if err != nil {
		resp.Diagnostics.AddError("Load Balancer Update Error", err.Error())
		return
	}

	if err := r.LoadBalancerRead(ctx, id, &plan, &resp.State); err != nil {
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

	_, _, err := r.M.Core.LoadBalancers.Delete(
		ctx,
		core.LoadBalancerRef{ID: state.ID.ValueString()},
	)
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
	state *tfsdk.State,
) error {
	lb, _, err := r.M.Core.LoadBalancers.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, katapult.ErrNotFound) {
			state.RemoveResource(ctx)

			return nil
		}

		return err
	}

	model.ID = types.StringValue(id)
	model.Name = types.StringValue(lb.Name)
	model.ResourceType = types.StringValue(string(lb.ResourceType))
	model.HTTPSRedirect = types.BoolValue(lb.HTTPSRedirect)
	if lb.IPAddress != nil {
		model.IPAddress = types.StringValue(lb.IPAddress.Address)
	}

	populateLoadBalancerTargets(model, lb.ResourceType, lb.ResourceIDs)

	return nil
}

func populateLoadBalancerTargets(
	model *LoadBalancerResourceModel,
	t core.ResourceType,
	ids []string,
) {
	list := flattenLoadBalancerResourceIDs(ids)
	model.VirtualMachineIDs = types.SetNull(types.StringType)
	model.TagIDs = types.SetNull(types.StringType)
	model.VirtualMachineGroupIDs = types.SetNull(types.StringType)

	switch t {
	case core.VirtualMachinesResourceType:
		model.VirtualMachineIDs = list
	case core.VirtualMachineGroupsResourceType:
		model.VirtualMachineGroupIDs = list
	case core.TagsResourceType:
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
) (core.ResourceType, []string) {
	var t core.ResourceType
	var list []attr.Value
	ids := []string{}

	switch {
	case !model.VirtualMachineIDs.IsUnknown():
		t = core.VirtualMachinesResourceType
		list = model.VirtualMachineIDs.Elements()
	case !model.VirtualMachineGroupIDs.IsUnknown():
		t = core.VirtualMachineGroupsResourceType
		list = model.VirtualMachineGroupIDs.Elements()
	case !model.TagIDs.IsUnknown():
		t = core.TagsResourceType
		list = model.TagIDs.Elements()
	}

	for _, item := range list {
		i := item.(types.String)

		ids = append(ids, i.ValueString())
	}

	return t, ids
}
