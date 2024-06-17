package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
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
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
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
	if res.StatusCode() < 200 || res.StatusCode() >= 300 {
		resp.Diagnostics.AddError(
			"Load Balancer Create Error",
			string(res.Body),
		)
		return
	}

	if res.JSON200.LoadBalancer.Id == nil {
		resp.Diagnostics.AddError(
			"Load Balancer Create Error",
			"missing ID in response",
		)
		return
	}

	id := *res.JSON200.LoadBalancer.Id

	if err := r.LoadBalancerRead(ctx, id, &plan, &resp.State); err != nil {
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
	} else {
		t, ids := extractLoadBalancerResourceTypeAndIDs(&state)
		args.Properties.ResourceType = &t
		args.Properties.ResourceIds = &ids
	}

	res, err := r.M.Core.PatchLoadBalancerWithResponse(ctx, args)
	if err != nil {
		resp.Diagnostics.AddError("Load Balancer Update Error", err.Error())
		return
	}

	if res.JSON200 == nil {
		resp.Diagnostics.AddError(
			"Load Balancer Update Error",
			"unexpected status code from API",
		)

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
	state *tfsdk.State,
) error {
	res, err := r.M.Core.GetLoadBalancerWithResponse(
		ctx,
		&core.GetLoadBalancerParams{
			LoadBalancerId: &id,
		})
	if err != nil {
		if res.JSON404 != nil {
			state.RemoveResource(ctx)

			return nil
		}

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
	model.VirtualMachineIDs = types.SetNull(types.StringType)
	model.TagIDs = types.SetNull(types.StringType)
	model.VirtualMachineGroupIDs = types.SetNull(types.StringType)

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
