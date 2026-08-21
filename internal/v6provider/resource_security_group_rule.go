package v6provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	Protocol        caseInsensitiveStringValue `tfsdk:"protocol"`
	Ports           types.String               `tfsdk:"ports"`
	Targets         types.Set                  `tfsdk:"targets"`
	Notes           types.String               `tfsdk:"notes"`
}

var (
	_ resource.ResourceWithImportState    = (*SecurityGroupRuleResource)(nil)
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
		"direction": schema.StringAttribute{
			Required: standalone, Computed: !standalone,
			MarkdownDescription: "The rule direction (`inbound` or `outbound`).",
			Validators:          []validator.String{stringvalidator.OneOfCaseInsensitive("inbound", "outbound")},
			PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"protocol": schema.StringAttribute{
			Required: true, MarkdownDescription: "The rule protocol (`TCP`, `UDP`, or `ICMP`).",
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
	if !config.Targets.IsNull() && !config.Targets.IsUnknown() {
		for _, target := range config.Targets.Elements() {
			value, ok := target.(types.String)
			if !ok || value.IsNull() || (!value.IsUnknown() && value.ValueString() == "") {
				resp.Diagnostics.AddAttributeError(path.Root("targets"), "Invalid Security Group Rule Target", "Targets cannot contain null or empty strings.")
				break
			}
		}
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
	protocol := core.SecurityGroupRuleProtocolEnum(strings.ToUpper(model.Protocol.ValueString()))
	args := core.SecurityGroupRuleArguments{Direction: &direction, Protocol: &protocol, Targets: &targets}
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
		plan.Direction = types.StringValue(strings.ToLower(string(*created.Direction)))
	}
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
	var plan SecurityGroupRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
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
	priorProtocol := model.Protocol.ValueString()
	model.ID = types.StringPointerValue(rule.Id)
	if rule.SecurityGroup != nil {
		model.SecurityGroupID = types.StringPointerValue(rule.SecurityGroup.Id)
	}
	if rule.Direction != nil {
		model.Direction = types.StringValue(strings.ToLower(string(*rule.Direction)))
	}
	if rule.Protocol != nil {
		remoteProtocol := strings.ToUpper(string(*rule.Protocol))
		if strings.EqualFold(priorProtocol, remoteProtocol) && priorProtocol != "" {
			remoteProtocol = priorProtocol
		}
		model.Protocol = caseInsensitiveStringValueOf(remoteProtocol)
	}
	model.Ports = types.StringValue(nullableString(rule.Ports))
	model.Notes = types.StringValue(nullableString(rule.Notes))
	if rule.Targets != nil {
		value, diags := types.SetValueFrom(ctx, types.StringType, *rule.Targets)
		if diags.HasError() {
			first := diags.Errors()[0]
			return false, fmt.Errorf("%s: %s", first.Summary(), first.Detail())
		}
		model.Targets = value
	}
	return false, nil
}
