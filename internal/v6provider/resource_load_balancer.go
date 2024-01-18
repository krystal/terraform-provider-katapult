package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
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
		t = core.VirtualMachines
	}

	// args := &core.LoadBalancerCreateArguments{
	// 	Name:         name,
	// 	ResourceType: t,
	// 	ResourceIDs:  &ids,
	// 	DataCenter:   r.M.DataCenterRef,
	// }

	// lb, _, err := r.M.Core.LoadBalancers.Create(
	// 	ctx, r.M.OrganizationRef, args,
	// )

	args := core.PostOrganizationLoadBalancersJSONRequestBody{
		Organization: core.OrganizationLookup{
			SubDomain: &r.M.OrganizationRef.SubDomain,
		},
		Properties: core.LoadBalancerArguments{
			Name:         &name,
			ResourceType: &t,
			ResourceIds:  &ids,
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

	if !plan.VirtualMachine.Equal(state.VirtualMachine) ||
		!plan.VirtualMachineGroup.Equal(state.VirtualMachineGroup) ||
		!plan.Tag.Equal(state.Tag) {
		t, ids := extractLoadBalancerResourceTypeAndIDs(&plan)
		args.Properties.ResourceType = &t
		args.Properties.ResourceIds = &ids
	}

	// _, _, err := r.M.Core.LoadBalancers.Update(ctx, lbLookup, args)
	_, err := r.M.Core.PatchLoadBalancerWithResponse(ctx, args)
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
	if lb.ResourceType != nil {
		model.ResourceType = types.StringValue(string(*lb.ResourceType))
	}
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
	case core.VirtualMachines:
		model.VirtualMachine = list
	case core.VirtualMachineGroups:
		model.VirtualMachineGroup = list
	case core.Tags:
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
) (core.LoadBalancerResourceTypesEnum, []string) {
	var t core.LoadBalancerResourceTypesEnum
	var list []attr.Value
	ids := []string{}

	switch {
	case !model.VirtualMachine.IsNull():
		t = core.VirtualMachines
		list = model.VirtualMachine.Elements()
	case !model.VirtualMachineGroup.IsNull():
		t = core.VirtualMachineGroups
		list = model.VirtualMachineGroup.Elements()
	case !model.Tag.IsNull():
		t = core.Tags
		list = model.Tag.Elements()
	}

	for _, item := range list {
		i := item.(types.Object)
		attrs := i.Attributes()

		ids = append(ids, attrs["id"].(types.String).ValueString())
	}

	return t, ids
}
