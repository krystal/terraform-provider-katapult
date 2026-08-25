package v6provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

type SecurityGroupRuleResource struct{ M *Meta }

type SecurityGroupRuleResourceModel struct {
	ID              types.String               `tfsdk:"id"`
	SecurityGroupID types.String               `tfsdk:"security_group_id"`
	Direction       types.String               `tfsdk:"direction"`
	Action          types.String               `tfsdk:"action"`
	Protocol        caseInsensitiveStringValue `tfsdk:"protocol"`
	Ports           types.String               `tfsdk:"ports"`
	Targets         types.Set                  `tfsdk:"targets"`
	Notes           types.String               `tfsdk:"notes"`
}

type securityGroupRuleResourceModelV0 struct {
	ID              types.String               `tfsdk:"id"`
	SecurityGroupID types.String               `tfsdk:"security_group_id"`
	Direction       types.String               `tfsdk:"direction"`
	Protocol        caseInsensitiveStringValue `tfsdk:"protocol"`
	Ports           types.String               `tfsdk:"ports"`
	Targets         types.Set                  `tfsdk:"targets"`
	Notes           types.String               `tfsdk:"notes"`
}

var (
	_ resource.ResourceWithImportState    = (*SecurityGroupRuleResource)(nil)
	_ resource.ResourceWithUpgradeState   = (*SecurityGroupRuleResource)(nil)
	_ resource.ResourceWithValidateConfig = (*SecurityGroupRuleResource)(nil)
)

//nolint:lll // Framework interface signature.
func (r *SecurityGroupRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_group_rule"
}

//nolint:lll // Framework interface signature.
func (r *SecurityGroupRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	m, ok := req.ProviderData.(*Meta)
	if !ok {
		resp.Diagnostics.AddError("Meta Error", "meta is not of type *Meta")
		return
	}
	r.M = m
}

//nolint:lll // Keep the complete shared rule schema together.
func securityGroupRuleAttributes(standalone bool) map[string]schema.Attribute {
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{Computed: true, MarkdownDescription: "The unique identifier of the security group rule.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		securityGroupDirectionJSONField: schema.StringAttribute{
			Required: standalone, Computed: !standalone,
			MarkdownDescription: "The rule direction (`inbound` or `outbound`). Comparisons are case-insensitive and configured casing is preserved when it matches the API value.",
			Validators:          []validator.String{stringvalidator.OneOfCaseInsensitive("inbound", "outbound")},
			PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		securityGroupActionJSONField: schema.StringAttribute{
			Optional: true, Computed: true,
			MarkdownDescription: "Whether the rule permits (`allow`) or drops (`deny`) matching traffic. Defaults to `allow`. Katapult evaluates all deny rules before allow rules, then applies an implicit deny-all rule. List order does not control evaluation.",
			Default:             stringdefault.StaticString(string(core.Allow)),
			Validators:          []validator.String{stringvalidator.OneOf(string(core.Allow), string(core.Deny))},
		},
		"protocol": schema.StringAttribute{
			Required: true, MarkdownDescription: "The rule protocol (`TCP`, `UDP`, or `ICMP`). Comparisons are case-insensitive. Existing and imported API casing remains stable, newly configured casing remains as written, and API requests use uppercase protocol values.",
			CustomType: caseInsensitiveStringType{},
			Validators: []validator.String{stringvalidator.OneOfCaseInsensitive("TCP", "UDP", "ICMP")},
			PlanModifiers: []planmodifier.String{
				caseInsensitiveUseStatePlanModifier{},
			},
		},
		"ports": schema.StringAttribute{
			Optional: true, Computed: true, MarkdownDescription: "The port, ports, or range of ports. Omit to match all ports.",
			Default: stringdefault.StaticString(""),
		},
		"targets": schema.SetAttribute{
			Required: true, ElementType: types.StringType,
			MarkdownDescription: "The target IPs, CIDRs, resource IDs, or `all:ipv4` and `all:ipv6` values.",
			Validators: []validator.Set{
				setvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
				setvalidator.NoNullValues(),
			},
		},
		"notes": schema.StringAttribute{
			Optional: true, Computed: true, MarkdownDescription: "Human-readable notes for the rule.",
			Default: stringdefault.StaticString(""),
		},
	}
	if standalone {
		attrs["security_group_id"] = schema.StringAttribute{
			Required: true, MarkdownDescription: "The security group to which the rule belongs.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			Validators:    []validator.String{stringvalidator.LengthAtLeast(1)},
		}
	}

	return attrs
}

type caseInsensitiveUseStatePlanModifier struct{}

func (caseInsensitiveUseStatePlanModifier) Description(context.Context) string {
	return "Preserves state casing when the configured value is equal ignoring case."
}

func (m caseInsensitiveUseStatePlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (caseInsensitiveUseStatePlanModifier) PlanModifyString(
	_ context.Context,
	req planmodifier.StringRequest,
	resp *planmodifier.StringResponse,
) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if req.StateValue.IsNull() || req.StateValue.IsUnknown() {
		return
	}
	if strings.EqualFold(req.ConfigValue.ValueString(), req.StateValue.ValueString()) {
		resp.PlanValue = req.StateValue
	}
}

//nolint:lll // Schema description is intentionally actionable.
func (r *SecurityGroupRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an individual security group rule. Use this with a group whose `external_rules` setting is enabled.",
		Attributes:          securityGroupRuleAttributes(true),
		Version:             1,
	}
}

//nolint:lll // The prior schema must remain explicit and reviewable beside its upgrader.
func (r *SecurityGroupRuleResource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
	priorAttributes := securityGroupRuleAttributes(true)
	delete(priorAttributes, securityGroupActionJSONField)
	priorSchema := schema.Schema{
		MarkdownDescription: "Manages an individual security group rule. Use this with a group whose `external_rules` setting is enabled.",
		Attributes:          priorAttributes,
	}
	return map[int64]resource.StateUpgrader{
		0: {
			PriorSchema: &priorSchema,
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				var prior securityGroupRuleResourceModelV0
				resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
				if resp.Diagnostics.HasError() {
					return
				}
				upgraded := SecurityGroupRuleResourceModel{
					ID: prior.ID, SecurityGroupID: prior.SecurityGroupID,
					Direction: prior.Direction, Action: types.StringValue(string(core.Allow)),
					Protocol: prior.Protocol, Ports: prior.Ports, Targets: prior.Targets, Notes: prior.Notes,
				}
				resp.Diagnostics.Append(resp.State.Set(ctx, &upgraded)...)
			},
		},
	}
}

//nolint:lll // Validation diagnostics remain readable and actionable.
func (r *SecurityGroupRuleResource) ValidateConfig(
	ctx context.Context,
	req resource.ValidateConfigRequest,
	resp *resource.ValidateConfigResponse,
) {
	var config SecurityGroupRuleResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !config.Protocol.IsNull() && !config.Protocol.IsUnknown() && strings.EqualFold(config.Protocol.ValueString(), "ICMP") &&
		!config.Ports.IsNull() && !config.Ports.IsUnknown() && config.Ports.ValueString() != "" {
		resp.Diagnostics.AddAttributeError(path.Root("ports"), "Invalid ICMP Rule Ports", "Ports cannot be set with ICMP.")
	}
}

//nolint:lll // API argument projection remains explicit.
func securityGroupRuleArguments(ctx context.Context, model SecurityGroupRuleResourceModel) (core.SecurityGroupRuleArguments, error) {
	var targets []string
	diags := model.Targets.ElementsAs(ctx, &targets, false)
	if diags.HasError() {
		first := diags.Errors()[0]
		return core.SecurityGroupRuleArguments{}, fmt.Errorf("%s: %s", first.Summary(), first.Detail())
	}
	direction := core.SecurityGroupRuleDirectionEnum(strings.ToLower(model.Direction.ValueString()))
	action := core.SecurityGroupRuleActionEnum(normalizeSecurityGroupRuleAction(model.Action.ValueString()))
	protocol := core.SecurityGroupRuleProtocolEnum(strings.ToUpper(model.Protocol.ValueString()))
	args := core.SecurityGroupRuleArguments{Action: &action, Direction: &direction, Protocol: &protocol, Targets: &targets}
	if !model.Ports.IsNull() && !model.Ports.IsUnknown() {
		args.Ports = model.Ports.ValueStringPointer()
	}
	if !model.Notes.IsNull() && !model.Notes.IsUnknown() {
		args.Notes = model.Notes.ValueStringPointer()
	}
	return args, nil
}

//nolint:lll // Framework lifecycle method.
func (r *SecurityGroupRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SecurityGroupRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	args, err := securityGroupRuleArguments(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Security Group Rule Create Error", err.Error())
		return
	}
	groupID := plan.SecurityGroupID.ValueString()
	res, err := r.M.Core.PostSecurityGroupRulesWithResponse(ctx, core.PostSecurityGroupRulesJSONRequestBody{
		SecurityGroup: core.SecurityGroupLookup{Id: &groupID}, Properties: args,
	})
	if err != nil {
		if res != nil {
			err = genericAPIError(err, res.Body)
		}
		resp.Diagnostics.AddError("Security Group Rule Create Error", err.Error())
		return
	}
	if res == nil || res.JSON200 == nil || res.JSON200.SecurityGroupRule.Id == nil {
		resp.Diagnostics.AddError("Security Group Rule Create Error", "unexpected create response")
		return
	}
	created := res.JSON200.SecurityGroupRule
	plan.ID = types.StringValue(*created.Id)
	if created.SecurityGroup != nil {
		plan.SecurityGroupID = types.StringPointerValue(created.SecurityGroup.Id)
	}
	if created.Direction != nil {
		remoteDirection := strings.ToLower(string(*created.Direction))
		plan.Direction = types.StringValue(preserveCaseInsensitiveCasing(
			plan.Direction.ValueString(), remoteDirection,
		))
	}
	plan.Action = types.StringValue(apiSecurityGroupRuleAction(created.Action))
	if created.Protocol != nil {
		plan.Protocol = caseInsensitiveStringValueOf(strings.ToUpper(string(*created.Protocol)))
	}
	plan.Ports = types.StringValue(nullableString(created.Ports))
	plan.Notes = types.StringValue(nullableString(created.Notes))
	targets := []string{}
	if created.Targets != nil {
		targets = append(targets, (*created.Targets)...)
	}
	plan.Targets, _ = types.SetValueFrom(ctx, types.StringType, targets)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SecurityGroupRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SecurityGroupRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	missing, err := r.readMaybeMissing(ctx, state.ID.ValueString(), &state)
	if err != nil {
		resp.Diagnostics.AddError("Security Group Rule Read Error", err.Error())
		return
	}
	if missing {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

//nolint:lll // Framework lifecycle method.
func (r *SecurityGroupRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state SecurityGroupRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if securityGroupRulePropertiesEqual(state, plan) {
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		return
	}
	args, err := securityGroupRuleArguments(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Security Group Rule Update Error", err.Error())
		return
	}
	id := plan.ID.ValueString()
	res, err := r.M.Core.PatchSecurityGroupsRulesSecurityGroupRuleWithResponse(ctx, core.PatchSecurityGroupsRulesSecurityGroupRuleJSONRequestBody{
		SecurityGroupRule: core.SecurityGroupRuleLookup{Id: &id}, Properties: args,
	})
	if err != nil {
		if res != nil {
			err = genericAPIError(err, res.Body)
		}
		resp.Diagnostics.AddError("Security Group Rule Update Error", err.Error())
		return
	}
	if res == nil || res.JSON200 == nil {
		resp.Diagnostics.AddError("Security Group Rule Update Error", "unexpected update response")
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func securityGroupRulePropertiesEqual(state, plan SecurityGroupRuleResourceModel) bool {
	return state.SecurityGroupID.Equal(plan.SecurityGroupID) &&
		strings.EqualFold(state.Direction.ValueString(), plan.Direction.ValueString()) &&
		state.Action.Equal(plan.Action) &&
		strings.EqualFold(state.Protocol.ValueString(), plan.Protocol.ValueString()) &&
		state.Ports.Equal(plan.Ports) &&
		state.Targets.Equal(plan.Targets) &&
		state.Notes.Equal(plan.Notes)
}

//nolint:lll,dupl // Idempotent rule deletion mirrors group deletion semantics.
func (r *SecurityGroupRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SecurityGroupRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.ID.ValueString()
	res, err := r.M.Core.DeleteSecurityGroupsRulesSecurityGroupRuleWithResponse(ctx, core.DeleteSecurityGroupsRulesSecurityGroupRuleJSONRequestBody{SecurityGroupRule: core.SecurityGroupRuleLookup{Id: &id}})
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return
		}
		if res != nil {
			err = genericAPIError(err, res.Body)
		}
		resp.Diagnostics.AddError("Security Group Rule Delete Error", err.Error())
		return
	}
	if res != nil && res.JSON404 != nil {
		return
	}
	if res == nil || (res.JSON200 == nil && res.StatusCode() != 204) {
		resp.Diagnostics.AddError("Security Group Rule Delete Error", "unexpected delete response")
	}
}

//nolint:lll // Framework interface signature.
func (r *SecurityGroupRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

//nolint:lll // Generated endpoint name and lookup type are necessarily verbose.
func (r *SecurityGroupRuleResource) readMaybeMissing(ctx context.Context, id string, model *SecurityGroupRuleResourceModel) (bool, error) {
	res, err := r.M.Core.GetSecurityGroupsRulesSecurityGroupRuleWithResponse(ctx, &core.GetSecurityGroupsRulesSecurityGroupRuleParams{SecurityGroupRuleId: &id})
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return true, nil
		}
		if res != nil {
			err = genericAPIError(err, res.Body)
		}
		return false, err
	}
	if res != nil && res.JSON404 != nil {
		return true, nil
	}
	if res == nil || res.JSON200 == nil {
		return false, core.ErrNotFound
	}
	rule := res.JSON200.SecurityGroupRule
	priorDirection := model.Direction.ValueString()
	priorProtocol := model.Protocol.ValueString()
	model.ID = types.StringPointerValue(rule.Id)
	if rule.SecurityGroup != nil {
		model.SecurityGroupID = types.StringPointerValue(rule.SecurityGroup.Id)
	}
	if rule.Direction != nil {
		model.Direction = types.StringValue(preserveCaseInsensitiveCasing(
			priorDirection, strings.ToLower(string(*rule.Direction)),
		))
	}
	model.Action = types.StringValue(apiSecurityGroupRuleAction(rule.Action))
	if rule.Protocol != nil {
		remoteProtocol := strings.ToUpper(string(*rule.Protocol))
		if strings.EqualFold(priorProtocol, remoteProtocol) && priorProtocol != "" {
			remoteProtocol = priorProtocol
		}
		model.Protocol = caseInsensitiveStringValueOf(remoteProtocol)
	}
	model.Ports = types.StringValue(nullableString(rule.Ports))
	model.Notes = types.StringValue(nullableString(rule.Notes))
	targets := []string{}
	if rule.Targets != nil {
		targets = *rule.Targets
	}
	value, diags := types.SetValueFrom(ctx, types.StringType, targets)
	if diags.HasError() {
		first := diags.Errors()[0]
		return false, fmt.Errorf("%s: %s", first.Summary(), first.Detail())
	}
	model.Targets = value
	return false, nil
}

func preserveCaseInsensitiveCasing(prior, remote string) string {
	if prior != "" && strings.EqualFold(prior, remote) {
		return prior
	}
	return remote
}
