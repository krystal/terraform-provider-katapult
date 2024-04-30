package v6provider

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework-validators/boolvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/krystal/go-katapult"
	"github.com/krystal/go-katapult/core"
)

type (
	LoadBalancerResource struct {
		M *Meta
	}

	LoadBalancerResourceModel struct {
		ID                   types.String `tfsdk:"id"`
		Name                 types.String `tfsdk:"name"`
		ResourceType         types.String `tfsdk:"resource_type"`
		VirtualMachines      types.List   `tfsdk:"virtual_machines"`
		VirtualMachineGroups types.List   `tfsdk:"virtual_machine_groups"`
		Tags                 types.List   `tfsdk:"tags"`
		IPAddress            types.String `tfsdk:"ip_address"`
		HTTPSRedirect        types.Bool   `tfsdk:"https_redirect"`
		ExternalRules        types.Bool   `tfsdk:"external_rules"`
		Rules                types.List   `tfsdk:"rules"`
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
			"virtual_machines": types.ListType{
				ElemType: types.StringType,
			},
			"virtual_machine_groups": types.ListType{
				ElemType: types.StringType,
			},
			"tags": types.ListType{
				ElemType: types.StringType,
			},
			"ip_address":     types.StringType,
			"https_redirect": types.BoolType,
			"rules": types.ListType{
				ElemType: LoadBalancerRuleType(),
			},
		},
	}
}

//nolint:funlen
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
			"virtual_machine": schema.ListAttribute{
				Optional: true,
				Computed: true,
				Validators: []validator.List{
					listvalidator.ConflictsWith(
						path.MatchRoot("tag"),
						path.MatchRoot("virtual_machine_group"),
					),
				},
				ElementType: types.StringType,
			},
			"virtual_machine_group": schema.ListAttribute{
				Optional: true,
				Computed: true,
				Validators: []validator.List{
					listvalidator.ConflictsWith(
						path.MatchRoot("tag"),
						path.MatchRoot("virtual_machine"),
					),
				},
				ElementType: types.StringType,
			},
			"tag": schema.ListAttribute{
				Optional: true,
				Computed: true,
				Validators: []validator.List{
					listvalidator.ConflictsWith(
						path.MatchRoot("virtual_machine"),
						path.MatchRoot("virtual_machine_group"),
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
			"external_rules": schema.BoolAttribute{
				Optional: true,
				Description: "When enabled, The full list of rules are not " +
					"managed by Terraform. Induvidual rules can still be " +
					"managed with the katapult_load_balancer_rule " +
					"resource. This is required to prevent Terraform from " +
					"deleting rules managed outside of Terraform. Defaults " +
					"to false.",
				MarkdownDescription: "When enabled, " +
					"The full list of rules are not " +
					"managed by Terraform. Induvidual rules can still be " +
					"managed with the `katapult_load_balancer_rule` " +
					"resource. This is required to prevent Terraform from " +
					"deleting rules managed outside of Terraform. Defaults " +
					"to `false`.",
				Validators: []validator.Bool{
					boolvalidator.ConflictsWith(path.MatchRoot("rules")),
				},
			},
			"rules": schema.ListNestedAttribute{
				Optional: true,
				Validators: []validator.List{
					listvalidator.ConflictsWith(
						path.MatchRoot("external_rules")),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: lbrSchema,
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

	if !plan.ExternalRules.ValueBool() &&
		(!plan.Rules.IsNull() || len(plan.Rules.Elements()) > 0) {
		rules := make(
			[]*LoadBalancerRuleResourceModel,
			len(plan.Rules.Elements()),
		)
		resp.Diagnostics.Append(plan.Rules.ElementsAs(ctx, &rules, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		for _, rule := range rules {
			lbr, createDiags := r.createRule(ctx, rule, lb.ID)
			if createDiags.HasError() {
				resp.Diagnostics.Append(createDiags...)
				return
			}

			rule.ID = types.StringValue(lbr.ID)
			rule.LoadBalancerID = types.StringValue(lb.ID)
		}
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

	if !plan.VirtualMachines.Equal(state.VirtualMachines) ||
		!plan.VirtualMachineGroups.Equal(state.VirtualMachineGroups) ||
		!plan.Tags.Equal(state.Tags) {
		t, ids := extractLoadBalancerResourceTypeAndIDs(&plan)
		args.ResourceType = t
		args.ResourceIDs = &ids
	}

	if !plan.ExternalRules.ValueBool() {
		create, update, del, diffDiags := diffLoadBalancerRules(
			ctx, state.Rules, plan.Rules)
		resp.Diagnostics.Append(diffDiags...)
		if resp.Diagnostics.HasError() {
			return
		}

		for _, rule := range create {
			lbr, createDiags := r.createRule(ctx, rule, id)
			if createDiags.HasError() {
				resp.Diagnostics.Append(createDiags...)
				return
			}

			rule.ID = types.StringValue(lbr.ID)
		}

		for _, rule := range update {
			_, updateDiags := r.updateRule(ctx, rule)

			if updateDiags.HasError() {
				resp.Diagnostics.Append(updateDiags...)
				return
			}
		}

		for _, rule := range del {
			_, _, err := r.M.Core.LoadBalancerRules.Delete(
				ctx, core.LoadBalancerRuleRef{
					ID: rule.ID.ValueString(),
				})
			if err != nil {
				resp.Diagnostics.AddError(
					"Load Balancer Rule Delete Error",
					err.Error(),
				)
				return
			}
		}
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
	if model.ExternalRules.ValueBool() {
		return nil
	}

	rules, err := getLBRules(ctx, r.M, lb.Ref())
	if err != nil {
		return err
	}

	// if model.Rules is null and len(rules) is 0, this is a null rules
	// if model.Rules is null and len(rules) is not 0,
	// this an import, we should set model.Rules
	// if model.Rules is not null then we should always set model.rules,
	// as this is either an update or create.
	if !model.Rules.IsNull() || len(rules) != 0 {
		model.Rules = types.ListValueMust(
			LoadBalancerRuleType(),
			convertCoreLBRulesToAttrValue(rules),
		)
	}

	return nil
}

func diffLoadBalancerRules(
	ctx context.Context,
	oldList basetypes.ListValue,
	newList basetypes.ListValue,
) (
	create, update, del []*LoadBalancerRuleResourceModel,
	diags diag.Diagnostics,
) {
	oldRules := make([]*LoadBalancerRuleResourceModel, len(oldList.Elements()))
	diags = oldList.ElementsAs(ctx, &oldRules, false)
	if diags.HasError() {
		return create, update, del, diags
	}

	newRules := make([]*LoadBalancerRuleResourceModel, len(newList.Elements()))
	diags = newList.ElementsAs(ctx, &newRules, false)
	if diags.HasError() {
		return create, update, del, diags
	}

	// Create a map of existing rules for easy lookup

	existing := map[string]*LoadBalancerRuleResourceModel{}
	for _, rule := range oldRules {
		existing[rule.ID.ValueString()] = rule
	}

	for _, rule := range newRules {
		if rule.ID.IsNull() || rule.ID.IsUnknown() {
			create = append(create, rule)
			continue
		}

		id := rule.ID.ValueString()

		oldRule, ok := existing[id]
		delete(existing, id)

		if !ok || diffLoadBalancerRule(oldRule, rule) {
			update = append(update, rule)
		}
	}

	for _, rule := range existing {
		del = append(del, rule)
	}

	return create, update, del, diags
}

//nolint:gocyclo // this has to check everything
func diffLoadBalancerRule(
	oldRule, newRule *LoadBalancerRuleResourceModel,
) bool {
	if oldRule == nil && newRule == nil {
		return false
	}

	if oldRule == nil || newRule == nil {
		return true
	}

	if !oldRule.Algorithm.Equal(newRule.Algorithm) ||
		!oldRule.Protocol.Equal(newRule.Protocol) ||
		!oldRule.ListenPort.Equal(newRule.ListenPort) ||
		!oldRule.DestinationPort.Equal(newRule.DestinationPort) ||
		!oldRule.ProxyProtocol.Equal(newRule.ProxyProtocol) ||
		!oldRule.BackendSSL.Equal(newRule.BackendSSL) ||
		!oldRule.PassthroughSSL.Equal(newRule.PassthroughSSL) ||
		!oldRule.CheckEnabled.Equal(newRule.CheckEnabled) ||
		!oldRule.CheckFall.Equal(newRule.CheckFall) ||
		!oldRule.CheckInterval.Equal(newRule.CheckInterval) ||
		!oldRule.CheckPath.Equal(newRule.CheckPath) ||
		!oldRule.CheckProtocol.Equal(newRule.CheckProtocol) ||
		!oldRule.CheckRise.Equal(newRule.CheckRise) ||
		!oldRule.CheckTimeout.Equal(newRule.CheckTimeout) ||
		!oldRule.CheckHTTPStatuses.Equal(newRule.CheckHTTPStatuses) ||
		!oldRule.Certificates.Equal(newRule.Certificates) {
		return true
	}

	return false
}

func (r *LoadBalancerResource) createRule(
	ctx context.Context,
	rule *LoadBalancerRuleResourceModel,
	lbID string,
) (*core.LoadBalancerRule, diag.Diagnostics) {
	var diags diag.Diagnostics
	args, diags := buildLoadBalancerRuleCreateArgs(ctx, rule)
	if diags.HasError() {
		return nil, diags
	}

	lbr, _, err := r.M.Core.LoadBalancerRules.Create(
		ctx, core.LoadBalancerRef{ID: lbID}, args)
	if err != nil {
		diags.AddError("Load Balancer Rule Create Error", err.Error())
		return nil, diags
	}

	return lbr, diags
}

func (r *LoadBalancerResource) updateRule(
	ctx context.Context,
	rule *LoadBalancerRuleResourceModel,
) (*core.LoadBalancerRule, diag.Diagnostics) {
	var diags diag.Diagnostics
	args, diags := buildLoadBalancerRuleCreateArgs(ctx, rule)
	if diags.HasError() {
		return nil, diags
	}

	lbr, _, err := r.M.Core.LoadBalancerRules.Update(
		ctx, core.LoadBalancerRuleRef{ID: rule.ID.ValueString()}, args)
	if err != nil {
		diags.AddError("Load Balancer Rule Update Error", err.Error())

		return nil, diags
	}

	return lbr, nil
}

func populateLoadBalancerTargets(
	model *LoadBalancerResourceModel,
	t core.ResourceType,
	ids []string,
) {
	list := flattenLoadBalancerResourceIDs(ids)
	model.VirtualMachines = types.ListNull(types.StringType)
	model.Tags = types.ListNull(types.StringType)
	model.VirtualMachineGroups = types.ListNull(types.StringType)

	switch t {
	case core.VirtualMachinesResourceType:
		model.VirtualMachines = list
	case core.VirtualMachineGroupsResourceType:
		model.VirtualMachineGroups = list
	case core.TagsResourceType:
		model.Tags = list
	}
}

func flattenLoadBalancerResourceIDs(ids []string) types.List {
	values := make([]attr.Value, len(ids))

	for i, id := range ids {
		values[i] = types.StringValue(id)
	}

	return types.ListValueMust(types.StringType, values)
}

func extractLoadBalancerResourceTypeAndIDs(
	model *LoadBalancerResourceModel,
) (core.ResourceType, []string) {
	var t core.ResourceType
	var list []attr.Value
	ids := []string{}

	switch {
	case !model.VirtualMachines.IsNull():
		t = core.VirtualMachinesResourceType
		list = model.VirtualMachines.Elements()
	case !model.VirtualMachineGroups.IsNull():
		t = core.VirtualMachineGroupsResourceType
		list = model.VirtualMachineGroups.Elements()
	case !model.Tags.IsNull():
		t = core.TagsResourceType
		list = model.Tags.Elements()
	}

	for _, item := range list {
		i := item.(types.String)

		ids = append(ids, i.ValueString())
	}

	return t, ids
}

func getLBRules(
	ctx context.Context,
	m *Meta,
	lbRef core.LoadBalancerRef,
) ([]*core.LoadBalancerRule, error) {
	var rules []*core.LoadBalancerRule

	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		pageResult, resp, err := m.Core.LoadBalancerRules.List(
			ctx, lbRef, &core.ListOptions{Page: pageNum},
		)
		if err != nil {
			return nil, err
		}

		totalPages = resp.Pagination.TotalPages
		rules = append(rules, pageResult...)
	}

	for i, rl := range rules {
		rule, _, err := m.Core.LoadBalancerRules.GetByID(
			ctx, rl.ID,
		)
		if err != nil {
			return nil, err
		}

		rules[i] = rule
	}

	return rules, nil
}

func convertCoreLBRulesToAttrValue(
	rules []*core.LoadBalancerRule,
) []attr.Value {
	attrs := make([]attr.Value, len(rules))
	for i, r := range rules {
		attrs[i] = types.ObjectValueMust(
			LoadBalancerRuleType().AttrTypes,
			map[string]attr.Value{
				"id":               types.StringValue(r.ID),
				"load_balancer_id": types.StringNull(),
				"algorithm":        types.StringValue(string(r.Algorithm)),
				"protocol":         types.StringValue(string(r.Protocol)),
				"listen_port":      types.Int64Value(int64(r.ListenPort)),
				"destination_port": types.Int64Value(int64(r.DestinationPort)),
				"proxy_protocol":   types.BoolValue(r.ProxyProtocol),
				"backend_ssl":      types.BoolValue(r.BackendSSL),
				"passthrough_ssl":  types.BoolValue(r.PassthroughSSL),
				"certificates": types.ListValueMust(
					CertificateType(),
					ConvertCoreCertsToTFValues(r.Certificates),
				),
				"check_enabled":  types.BoolValue(r.CheckEnabled),
				"check_fall":     types.Int64Value(int64(r.CheckFall)),
				"check_interval": types.Int64Value(int64(r.CheckInterval)),
				"check_path":     types.StringValue(r.CheckPath),
				"check_protocol": types.StringValue(string(r.CheckProtocol)),
				"check_rise":     types.Int64Value(int64(r.CheckRise)),
				"check_timeout":  types.Int64Value(int64(r.CheckTimeout)),
				"check_http_statuses": types.StringValue(
					string(r.CheckHTTPStatuses),
				),
			},
		)
	}

	return attrs
}
