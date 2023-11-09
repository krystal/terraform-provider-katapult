package v6provider

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
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
		ID                  types.String `tfsdk:"id"`
		Name                types.String `tfsdk:"name"`
		ResourceType        types.String `tfsdk:"resource_type"`
		VirtualMachine      types.List   `tfsdk:"virtual_machine"`
		VirtualMachineGroup types.List   `tfsdk:"virtual_machine_group"`
		Tag                 types.List   `tfsdk:"tag"`
		IPAddress           types.String `tfsdk:"ip_address"`
		HTTPSRedirect       types.Bool   `tfsdk:"https_redirect"`
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
				Computed: true,
				Optional: true,
			},
			"resource_type": schema.StringAttribute{
				Computed: true,
			},
			"virtual_machine": schema.ListNestedAttribute{
				Optional: true,
				Computed: true,
				Validators: []validator.List{
					listvalidator.ConflictsWith(
						path.MatchRoot("tag"),
						path.MatchRoot("virtual_machine_group"),
					),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Required: true,
						},
					},
				},
			},
			"virtual_machine_group": schema.ListNestedAttribute{
				Optional: true,
				Computed: true,
				Validators: []validator.List{
					listvalidator.ConflictsWith(
						path.MatchRoot("tag"),
						path.MatchRoot("virtual_machine"),
					),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Required: true,
						},
					},
				},
			},
			"tag": schema.ListNestedAttribute{
				Optional: true,
				Computed: true,
				Validators: []validator.List{
					listvalidator.ConflictsWith(
						path.MatchRoot("virtual_machine"),
						path.MatchRoot("virtual_machine_group"),
					),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Required: true,
						},
					},
				},
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

	plan.ID = types.StringValue(lb.ID)

	if err := r.LoadBalancerRead(ctx, &plan, &resp.State); err != nil {
		resp.Diagnostics.AddError("Load Balancer Read Error", err.Error())
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
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

	if err := r.LoadBalancerRead(ctx, state, &resp.State); err != nil {
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

	var id string
	if !plan.ID.IsNull() && plan.ID.ValueString() != "" {
		id = plan.ID.ValueString()
	} else {
		id = state.ID.ValueString()
		plan.ID = state.ID
	}

	lbRef := core.LoadBalancerRef{ID: id}
	args := &core.LoadBalancerUpdateArguments{}

	if !plan.Name.Equal(state.Name) {
		args.Name = plan.Name.ValueString()
	}

	if !plan.VirtualMachine.Equal(state.VirtualMachine) ||
		!plan.VirtualMachineGroup.Equal(state.VirtualMachineGroup) ||
		!plan.Tag.Equal(state.Tag) {
		t, ids := extractLoadBalancerResourceTypeAndIDs(&plan)
		args.ResourceType = t
		args.ResourceIDs = &ids
	}

	_, _, err := r.M.Core.LoadBalancers.Update(ctx, lbRef, args)
	if err != nil {
		resp.Diagnostics.AddError("Load Balancer Update Error", err.Error())
		return
	}

	if err := r.LoadBalancerRead(ctx, &plan, &resp.State); err != nil {
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
	model *LoadBalancerResourceModel,
	state *tfsdk.State,
) error {
	lb, _, err := r.M.Core.LoadBalancers.GetByID(ctx, model.ID.ValueString())
	if err != nil {
		if errors.Is(err, katapult.ErrNotFound) {
			state.RemoveResource(ctx)

			return nil
		}

		return err
	}

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
	model.VirtualMachine = types.ListNull(types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"id": types.StringType,
		},
	})
	model.Tag = types.ListNull(types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"id": types.StringType,
		},
	})
	model.VirtualMachineGroup = types.ListNull(types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"id": types.StringType,
		},
	})

	switch t {
	case core.VirtualMachinesResourceType:
		model.VirtualMachine = list
	case core.VirtualMachineGroupsResourceType:
		model.VirtualMachineGroup = list
	case core.TagsResourceType:
		model.Tag = list
	}
}

func flattenLoadBalancerResourceIDs(ids []string) types.List {
	values := make([]attr.Value, len(ids))

	for i, id := range ids {
		values[i] = types.ObjectValueMust(map[string]attr.Type{
			"id": types.StringType,
		}, map[string]attr.Value{
			"id": types.StringValue(id),
		})
	}

	return types.ListValueMust(types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"id": types.StringType,
		},
	}, values)
}

func extractLoadBalancerResourceTypeAndIDs(
	model *LoadBalancerResourceModel,
) (core.ResourceType, []string) {
	var t core.ResourceType
	var list []attr.Value
	ids := []string{}

	switch {
	case !model.VirtualMachine.IsNull():
		t = core.VirtualMachinesResourceType
		list = model.VirtualMachine.Elements()
	case !model.VirtualMachineGroup.IsNull():
		t = core.VirtualMachineGroupsResourceType
		list = model.VirtualMachineGroup.Elements()
	case !model.Tag.IsNull():
		t = core.TagsResourceType
		list = model.Tag.Elements()
	}

	for _, item := range list {
		i := item.(types.Object)
		attrs := i.Attributes()

		ids = append(ids, attrs["id"].(types.String).ValueString())
	}

	return t, ids
}
