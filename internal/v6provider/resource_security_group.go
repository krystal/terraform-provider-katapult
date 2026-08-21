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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

type SecurityGroupResource struct{ M *Meta }

const (
	securityGroupImportPrivateKey           = "security_group_import"
	securityGroupExternalAdoptionPrivateKey = "security_group_external_adoption_pending"
)

type SecurityGroupResourceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Associations     types.Set    `tfsdk:"associations"`
	AllowAllInbound  types.Bool   `tfsdk:"allow_all_inbound"`
	AllowAllOutbound types.Bool   `tfsdk:"allow_all_outbound"`
	ExternalRules    types.Bool   `tfsdk:"external_rules"`
	InboundRules     types.List   `tfsdk:"inbound_rules"`
	OutboundRules    types.List   `tfsdk:"outbound_rules"`
	InboundRule      types.List   `tfsdk:"inbound_rule"`
	OutboundRule     types.List   `tfsdk:"outbound_rule"`
}

var (
	_ resource.ResourceWithImportState    = (*SecurityGroupResource)(nil)
	_ resource.ResourceWithModifyPlan     = (*SecurityGroupResource)(nil)
	_ resource.ResourceWithValidateConfig = (*SecurityGroupResource)(nil)
)

//nolint:lll // Framework interface signature.
func (r *SecurityGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_group"
}

//nolint:lll // Framework interface signature.
func (r *SecurityGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func securityGroupRuleNestedAttributes() map[string]schema.Attribute {
	return securityGroupRuleAttributes(false)
}

//nolint:lll // Keep the complete compatibility schema together.
func (r *SecurityGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	ruleDescription := "Rules managed as an expression-friendly list."
	deprecated := "Use the corresponding plural rules attribute. This block remains supported for backwards compatibility."
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a security group, its associations, and optionally its complete inline rule set.",
		Attributes: map[string]schema.Attribute{
			"id":                               schema.StringAttribute{Computed: true, MarkdownDescription: "The unique identifier of the security group.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"name":                             schema.StringAttribute{Required: true, MarkdownDescription: "The security group name.", Validators: []validator.String{stringvalidator.LengthAtLeast(1)}},
			securityGroupAssociationsAttribute: schema.SetAttribute{Optional: true, Computed: true, ElementType: types.StringType, MarkdownDescription: "Resource IDs to which the group applies.", PlanModifiers: []planmodifier.Set{NullToEmptySetPlanModifier(), setplanmodifier.UseStateForUnknown()}},
			"allow_all_inbound":                schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Allow all inbound traffic."},
			"allow_all_outbound":               schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Allow all outbound traffic."},
			"external_rules":                   schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Do not manage the group's complete rule list inline. Standalone rule resources can still be used.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"inbound_rules":                    schema.ListNestedAttribute{Optional: true, Computed: true, MarkdownDescription: "Inbound " + ruleDescription, NestedObject: schema.NestedAttributeObject{Attributes: securityGroupRuleNestedAttributes()}},
			"outbound_rules":                   schema.ListNestedAttribute{Optional: true, Computed: true, MarkdownDescription: "Outbound " + ruleDescription, NestedObject: schema.NestedAttributeObject{Attributes: securityGroupRuleNestedAttributes()}},
		},
		Blocks: map[string]schema.Block{
			"inbound_rule":  schema.ListNestedBlock{MarkdownDescription: "Deprecated inbound rule blocks.", DeprecationMessage: deprecated, NestedObject: schema.NestedBlockObject{Attributes: securityGroupRuleNestedAttributes()}},
			"outbound_rule": schema.ListNestedBlock{MarkdownDescription: "Deprecated outbound rule blocks.", DeprecationMessage: deprecated, NestedObject: schema.NestedBlockObject{Attributes: securityGroupRuleNestedAttributes()}},
		},
	}
}

func knownCollectionConfigured(value types.List) bool { return !value.IsNull() && !value.IsUnknown() }

func knownCollectionHasValues(value types.List) bool {
	return knownCollectionConfigured(value) && len(value.Elements()) > 0
}

func legacyRepresentationForDirection(model SecurityGroupResourceModel, direction string) bool {
	if direction == securityGroupDirectionInbound {
		return knownCollectionConfigured(model.InboundRule) ||
			(model.InboundRule.IsUnknown() && model.InboundRules.IsNull())
	}

	return knownCollectionConfigured(model.OutboundRule) ||
		(model.OutboundRule.IsUnknown() && model.OutboundRules.IsNull())
}

//nolint:lll // Diagnostics remain actionable at each compatibility check.
func (r *SecurityGroupResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var conf SecurityGroupResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &conf)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for _, direction := range []struct {
		name        string
		attr, block types.List
		allow       types.Bool
	}{
		{securityGroupDirectionInbound, conf.InboundRules, conf.InboundRule, conf.AllowAllInbound},
		{securityGroupDirectionOutbound, conf.OutboundRules, conf.OutboundRule, conf.AllowAllOutbound},
	} {
		if knownCollectionConfigured(direction.attr) && knownCollectionConfigured(direction.block) {
			resp.Diagnostics.AddAttributeError(path.Root(direction.name+"_rules"), "Conflicting Security Group Rule Representations", "Configure either the plural rules attribute or the deprecated singular rule blocks for this direction, not both.")
		}
		if !direction.allow.IsNull() && !direction.allow.IsUnknown() && direction.allow.ValueBool() && (knownCollectionHasValues(direction.attr) || knownCollectionHasValues(direction.block)) {
			resp.Diagnostics.AddAttributeError(path.Root("allow_all_"+direction.name), "Conflicting Security Group Rules", "Allow-all cannot be enabled while rules are configured for the same direction.")
		}
	}
	if !conf.ExternalRules.IsNull() && !conf.ExternalRules.IsUnknown() && conf.ExternalRules.ValueBool() &&
		(knownCollectionConfigured(conf.InboundRules) || knownCollectionConfigured(conf.OutboundRules) || knownCollectionConfigured(conf.InboundRule) || knownCollectionConfigured(conf.OutboundRule)) {
		resp.Diagnostics.AddAttributeError(path.Root("external_rules"), "Conflicting Security Group Rules", "external_rules cannot be enabled while an inline rule collection is configured.")
	}
}

func listRules(ctx context.Context, value types.List, direction string) ([]canonicalSecurityGroupRule, error) {
	rules, diags := canonicalRulesFromList(ctx, value, direction)
	if diags.HasError() {
		first := diags.Errors()[0]
		return nil, fmt.Errorf("%s: %s", first.Summary(), first.Detail())
	}
	return rules, nil
}

func configuredList(ctx context.Context, req resource.ModifyPlanRequest, name string) (types.List, error) {
	var value types.List
	diags := req.Config.GetAttribute(ctx, path.Root(name), &value)
	if diags.HasError() {
		first := diags.Errors()[0]
		return value, fmt.Errorf("%s: %s", first.Summary(), first.Detail())
	}
	return value, nil
}

//nolint:lll,gocyclo,gocritic,funlen // Directional migration follows contract order.
func (r *SecurityGroupResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || req.State.Raw.IsNull() {
		return
	}
	var plan, state SecurityGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var configuredExternalRules types.Bool
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("external_rules"), &configuredExternalRules)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if configuredExternalRules.IsNull() && !state.ExternalRules.IsNull() && !state.ExternalRules.IsUnknown() && state.ExternalRules.ValueBool() {
		plan.ExternalRules = types.BoolValue(false)
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("external_rules"), plan.ExternalRules)...)
	}
	adoptingExternalRules := !state.ExternalRules.IsNull() && !state.ExternalRules.IsUnknown() &&
		state.ExternalRules.ValueBool() && !plan.ExternalRules.IsUnknown() && !plan.ExternalRules.ValueBool()
	externalAdoptionPending := false
	if req.Private != nil {
		value, diags := req.Private.GetKey(ctx, securityGroupExternalAdoptionPrivateKey)
		resp.Diagnostics.Append(diags...)
		externalAdoptionPending = len(value) > 0
		if resp.Diagnostics.HasError() {
			return
		}
	}
	pendingReconciliation, pendingDeferred := false, false

	for _, direction := range []string{securityGroupDirectionInbound, securityGroupDirectionOutbound} {
		attrName, blockName := direction+"_rules", direction+"_rule"
		configuredAttr, err := configuredList(ctx, req, attrName)
		if err != nil {
			resp.Diagnostics.AddError("Security Group Plan Error", err.Error())
			return
		}
		configuredBlock, err := configuredList(ctx, req, blockName)
		if err != nil {
			resp.Diagnostics.AddError("Security Group Plan Error", err.Error())
			return
		}
		if configuredAttr.IsUnknown() || configuredBlock.IsUnknown() {
			// Dependency-driven attributes and dynamic blocks have not selected
			// a representation or desired collection yet. Preserve Terraform's
			// unknown plan so apply can evaluate the resolved configuration.
			pendingDeferred = pendingDeferred || externalAdoptionPending
			continue
		}
		attrSelected := knownCollectionConfigured(configuredAttr)
		blockSelected := knownCollectionConfigured(configuredBlock)
		representationUnconfigured := !attrSelected && !blockSelected
		priorLegacy := legacyRepresentationForDirection(state, direction)
		if representationUnconfigured {
			// An omitted deprecated block means removal. Project the resulting
			// empty managed collection through the primary plural attribute;
			// Framework cannot persist an empty nested block distinctly from null.
			blockSelected = false
			attrSelected = true
		}
		selectedPlan := plan.OutboundRules
		if direction == securityGroupDirectionInbound {
			selectedPlan = plan.InboundRules
		}
		if blockSelected {
			selectedPlan = plan.OutboundRule
			if direction == securityGroupDirectionInbound {
				selectedPlan = plan.InboundRule
			}
		}
		if adoptingExternalRules {
			selectedName, unusedName := attrName, blockName
			selectedValue := types.ListUnknown(securityGroupRuleObjectType())
			if blockSelected {
				selectedName, unusedName = blockName, attrName
				value, diags := securityGroupRuleBlockPlanValue(selectedPlan)
				resp.Diagnostics.Append(diags...)
				selectedValue = value
			}
			resp.Diagnostics.Append(resp.Plan.SetAttribute(
				ctx, path.Root(selectedName), selectedValue,
			)...)
			resp.Diagnostics.Append(resp.Plan.SetAttribute(
				ctx, path.Root(unusedName), types.ListNull(securityGroupRuleObjectType()),
			)...)
			continue
		}
		if !plan.ExternalRules.IsNull() && !plan.ExternalRules.IsUnknown() && plan.ExternalRules.ValueBool() {
			resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(attrName), types.ListNull(securityGroupRuleObjectType()))...)
			resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(blockName), types.ListNull(securityGroupRuleObjectType()))...)
			continue
		}
		var prior, desired []canonicalSecurityGroupRule
		var desiredErr error
		if direction == securityGroupDirectionInbound {
			if knownCollectionConfigured(state.InboundRule) {
				prior, _ = listRules(ctx, state.InboundRule, direction)
			} else {
				prior, _ = listRules(ctx, state.InboundRules, direction)
			}
			if representationUnconfigured {
				if externalAdoptionPending {
					desired = nil
					pendingReconciliation = pendingReconciliation || len(prior) > 0
				} else if priorLegacy {
					desired = nil
				} else {
					desired = prior
				}
			}
		} else {
			if knownCollectionConfigured(state.OutboundRule) {
				prior, _ = listRules(ctx, state.OutboundRule, direction)
			} else {
				prior, _ = listRules(ctx, state.OutboundRules, direction)
			}
			if representationUnconfigured {
				if externalAdoptionPending {
					desired = nil
					pendingReconciliation = pendingReconciliation || len(prior) > 0
				} else if priorLegacy {
					desired = nil
				} else {
					desired = prior
				}
			}
		}
		if !representationUnconfigured && securityGroupRuleListHasUnknownConfiguredFields(selectedPlan) {
			value, diags := securityGroupRuleBlockPlanValue(selectedPlan)
			resp.Diagnostics.Append(diags...)
			if blockSelected {
				resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(blockName), value)...)
				resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(attrName), types.ListNull(securityGroupRuleObjectType()))...)
			} else {
				resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(attrName), value)...)
				resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(blockName), types.ListNull(securityGroupRuleObjectType()))...)
			}
			pendingDeferred = pendingDeferred || externalAdoptionPending
			continue
		}
		if !representationUnconfigured {
			desired, desiredErr = listRules(ctx, selectedPlan, direction)
		}
		if representationUnconfigured && priorLegacy && len(prior) == 0 {
			// SDKv2 records an empty compatibility block collection for groups
			// without rules. Leaving Terraform's raw plan untouched hands that
			// empty state over without introducing plural empty-list churn.
			continue
		}
		if representationUnconfigured && securityGroupRuleCollectionsAmbiguous(state) && len(prior) == 0 {
			null := types.ListNull(securityGroupRuleObjectType())
			resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(blockName), null)...)
			resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(attrName), null)...)
			continue
		}
		if desiredErr != nil {
			if blockSelected {
				value, diags := securityGroupRuleBlockPlanValue(selectedPlan)
				resp.Diagnostics.Append(diags...)
				resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(blockName), value)...)
				resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(attrName), types.ListNull(securityGroupRuleObjectType()))...)
			}
			// Unknown configured values cannot be canonically paired yet.
			continue
		}
		desired = transferSecurityGroupRuleIDs(
			prior, desired, !representationUnconfigured,
		)
		if blockSelected {
			unmatched := false
			for _, rule := range desired {
				if rule.ID == "" {
					unmatched = true
					break
				}
			}
			if unmatched {
				value, diags := securityGroupRuleBlockPlanValue(selectedPlan)
				resp.Diagnostics.Append(diags...)
				resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(blockName), value)...)
				resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(attrName), types.ListNull(securityGroupRuleObjectType()))...)
				continue
			}
		}
		value, diags := securityGroupRulesPlanListValue(ctx, desired)
		resp.Diagnostics.Append(diags...)
		if attrSelected {
			resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(attrName), value)...)
			resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(blockName), types.ListNull(securityGroupRuleObjectType()))...)
		} else {
			resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(blockName), value)...)
			resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(attrName), types.ListNull(securityGroupRuleObjectType()))...)
		}
	}
	if externalAdoptionPending && !pendingReconciliation && !pendingDeferred && resp.Private != nil {
		resp.Diagnostics.Append(resp.Private.SetKey(ctx, securityGroupExternalAdoptionPrivateKey, nil)...)
	}
}

//nolint:lll // API argument projection remains explicit.
func securityGroupArguments(ctx context.Context, model SecurityGroupResourceModel) (core.SecurityGroupArguments, error) {
	associations := []string{}
	if !model.Associations.IsNull() && !model.Associations.IsUnknown() {
		diags := model.Associations.ElementsAs(ctx, &associations, false)
		if diags.HasError() {
			first := diags.Errors()[0]
			return core.SecurityGroupArguments{}, fmt.Errorf("%s: %s", first.Summary(), first.Detail())
		}
	}
	return core.SecurityGroupArguments{Name: model.Name.ValueStringPointer(), Associations: &associations, AllowAllInbound: model.AllowAllInbound.ValueBoolPointer(), AllowAllOutbound: model.AllowAllOutbound.ValueBoolPointer()}, nil
}

//nolint:lll // Representation selection returns canonical rules and source shape.
func selectedRules(ctx context.Context, model SecurityGroupResourceModel, direction string) ([]canonicalSecurityGroupRule, bool, error) {
	if direction == securityGroupDirectionInbound {
		if knownCollectionConfigured(model.InboundRule) {
			rules, err := listRules(ctx, model.InboundRule, direction)
			return rules, true, err
		}
		rules, err := listRules(ctx, model.InboundRules, direction)
		return rules, false, err
	}
	if knownCollectionConfigured(model.OutboundRule) {
		rules, err := listRules(ctx, model.OutboundRule, direction)
		return rules, true, err
	}
	rules, err := listRules(ctx, model.OutboundRules, direction)
	return rules, false, err
}

//nolint:lll,gocyclo // Partial-state persistence and rule creation are intentionally adjacent.
func (r *SecurityGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SecurityGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	args, err := securityGroupArguments(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Security Group Create Error", err.Error())
		return
	}
	res, err := r.M.Core.PostOrganizationSecurityGroupsWithResponse(ctx, core.PostOrganizationSecurityGroupsJSONRequestBody{Organization: core.OrganizationLookup{SubDomain: &r.M.confOrganization}, Properties: args})
	if err != nil {
		if res != nil {
			err = genericAPIError(err, res.Body)
		}
		resp.Diagnostics.AddError("Security Group Create Error", err.Error())
		return
	}
	if res == nil || res.JSON200 == nil || res.JSON200.SecurityGroup.Id == nil {
		resp.Diagnostics.AddError("Security Group Create Error", "unexpected create response")
		return
	}
	plan.ID = types.StringValue(*res.JSON200.SecurityGroup.Id)
	if plan.Associations.IsNull() || plan.Associations.IsUnknown() {
		associations, diags := types.SetValueFrom(ctx, types.StringType, []string{})
		resp.Diagnostics.Append(diags...)
		plan.Associations = associations
	}
	if plan.ExternalRules.IsNull() || plan.ExternalRules.IsUnknown() {
		plan.ExternalRules = types.BoolValue(false)
	}
	if plan.ExternalRules.ValueBool() {
		null := types.ListNull(securityGroupRuleObjectType())
		plan.InboundRules, plan.OutboundRules = null, null
		plan.InboundRule, plan.OutboundRule = null, null
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		return
	}
	for _, direction := range []string{securityGroupDirectionInbound, securityGroupDirectionOutbound} {
		rules, legacy, ruleErr := selectedRules(ctx, plan, direction)
		if ruleErr != nil {
			resp.Diagnostics.AddError("Security Group Rule Create Error", ruleErr.Error())
			return
		}
		if err := setSecurityGroupRules(ctx, &plan, direction, legacy, rules); err != nil {
			resp.Diagnostics.AddError("Security Group State Error", err.Error())
			return
		}
	}
	// Persist the group before inline rule creation so a later rule failure does
	// not orphan the already-created remote resource.
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for _, direction := range []string{securityGroupDirectionInbound, securityGroupDirectionOutbound} {
		rules, legacy, ruleErr := selectedRules(ctx, plan, direction)
		if ruleErr != nil {
			resp.Diagnostics.AddError("Security Group Rule Create Error", ruleErr.Error())
			return
		}
		rules, ruleErr = r.reconcileRules(ctx, plan.ID.ValueString(), nil, rules)
		if ruleErr != nil {
			resp.Diagnostics.AddError("Security Group Rule Create Error", ruleErr.Error())
			return
		}
		if err := setSecurityGroupRules(ctx, &plan, direction, legacy, rules); err != nil {
			resp.Diagnostics.AddError("Security Group State Error", err.Error())
			return
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SecurityGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SecurityGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ambiguousRules := securityGroupRuleCollectionsAmbiguous(state)
	imported, privateDiags := req.Private.GetKey(ctx, securityGroupImportPrivateKey)
	resp.Diagnostics.Append(privateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	missing, err := r.readMaybeMissing(ctx, &state)
	if err != nil {
		resp.Diagnostics.AddError("Security Group Read Error", err.Error())
		return
	}
	if missing {
		resp.State.RemoveResource(ctx)
		return
	}
	if ambiguousRules && len(imported) == 0 && securityGroupRuleCollectionsEmpty(state) {
		null := types.ListNull(securityGroupRuleObjectType())
		state.InboundRule, state.OutboundRule = null, null
		state.InboundRules, state.OutboundRules = null, null
	}
	if len(imported) > 0 && resp.Private != nil {
		resp.Diagnostics.Append(resp.Private.SetKey(ctx, securityGroupImportPrivateKey, nil)...)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func securityGroupRuleCollectionsAmbiguous(model SecurityGroupResourceModel) bool {
	return !knownCollectionConfigured(model.InboundRule) &&
		!knownCollectionConfigured(model.OutboundRule) &&
		!knownCollectionConfigured(model.InboundRules) &&
		!knownCollectionConfigured(model.OutboundRules)
}

func securityGroupRuleCollectionsEmpty(model SecurityGroupResourceModel) bool {
	return !knownCollectionHasValues(model.InboundRule) &&
		!knownCollectionHasValues(model.OutboundRule) &&
		!knownCollectionHasValues(model.InboundRules) &&
		!knownCollectionHasValues(model.OutboundRules)
}

//nolint:lll,gocyclo,gocritic // External-rule lifecycle branches stay together.
func (r *SecurityGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state SecurityGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	args, err := securityGroupArguments(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Security Group Update Error", err.Error())
		return
	}
	id := state.ID.ValueString()
	if !securityGroupPropertiesEqual(state, plan) {
		res, err := r.M.Core.PatchSecurityGroupWithResponse(ctx, core.PatchSecurityGroupJSONRequestBody{SecurityGroup: core.SecurityGroupLookup{Id: &id}, Properties: args})
		if err != nil {
			if res != nil {
				err = genericAPIError(err, res.Body)
			}
			resp.Diagnostics.AddError("Security Group Update Error", err.Error())
			return
		}
		if res == nil || res.JSON200 == nil {
			resp.Diagnostics.AddError("Security Group Update Error", "unexpected update response")
			return
		}
	}
	wasExternal := state.ExternalRules.ValueBool()
	isExternal := plan.ExternalRules.ValueBool()
	externalAdoptionPending := false
	if req.Private != nil {
		value, privateDiags := req.Private.GetKey(ctx, securityGroupExternalAdoptionPrivateKey)
		resp.Diagnostics.Append(privateDiags...)
		externalAdoptionPending = len(value) > 0
		if resp.Diagnostics.HasError() {
			return
		}
	}
	if !wasExternal && isExternal {
		for _, direction := range []string{securityGroupDirectionInbound, securityGroupDirectionOutbound} {
			prior, _, ruleErr := selectedRules(ctx, state, direction)
			if ruleErr == nil {
				_, ruleErr = r.reconcileRules(ctx, id, prior, nil)
			}
			if ruleErr != nil {
				resp.Diagnostics.AddError("Security Group Rule Update Error", ruleErr.Error())
				return
			}
		}
	} else if wasExternal && !isExternal {
		resp.Diagnostics.AddWarning("Security Group external_rules Disabled", "Run terraform plan or apply again to reconcile externally managed rules with Terraform configuration.")
	} else if !isExternal {
		for _, direction := range []string{securityGroupDirectionInbound, securityGroupDirectionOutbound} {
			prior, _, ruleErr := selectedRules(ctx, state, direction)
			desired, legacy, desiredErr := selectedRules(ctx, plan, direction)
			if ruleErr != nil {
				desiredErr = ruleErr
			}
			if desiredErr != nil {
				resp.Diagnostics.AddError("Security Group Rule Update Error", desiredErr.Error())
				return
			}
			desired, desiredErr = r.reconcileRules(ctx, id, prior, desired)
			if desiredErr != nil {
				resp.Diagnostics.AddError("Security Group Rule Update Error", desiredErr.Error())
				return
			}
			if err := setSecurityGroupRules(ctx, &plan, direction, legacy, desired); err != nil {
				resp.Diagnostics.AddError("Security Group State Error", err.Error())
				return
			}
		}
	}
	if wasExternal && !isExternal {
		configuredPlan := plan
		if err := r.read(ctx, &plan); err != nil {
			resp.Diagnostics.AddError("Security Group Read Error", err.Error())
			return
		}
		adoptedRulesPresent := !securityGroupRuleCollectionsEmpty(plan)
		for _, direction := range []string{securityGroupDirectionInbound, securityGroupDirectionOutbound} {
			if !legacyRepresentationForDirection(configuredPlan, direction) {
				continue
			}
			adopted, _, adoptedErr := selectedRules(ctx, plan, direction)
			configured, _, configuredErr := selectedRules(ctx, configuredPlan, direction)
			if adoptedErr != nil {
				configuredErr = adoptedErr
			}
			if configuredErr != nil {
				resp.Diagnostics.AddError("Security Group State Error", configuredErr.Error())
				return
			}
			configured = transferSecurityGroupRuleIDs(adopted, configured, true)
			if err := setSecurityGroupRules(ctx, &plan, direction, true, configured); err != nil {
				resp.Diagnostics.AddError("Security Group State Error", err.Error())
				return
			}
		}
		if resp.Private != nil && adoptedRulesPresent {
			resp.Diagnostics.Append(resp.Private.SetKey(ctx, securityGroupExternalAdoptionPrivateKey, []byte("true"))...)
		} else if resp.Private != nil {
			resp.Diagnostics.Append(resp.Private.SetKey(ctx, securityGroupExternalAdoptionPrivateKey, nil)...)
		}
	} else if externalAdoptionPending && !isExternal && resp.Private != nil {
		resp.Diagnostics.Append(resp.Private.SetKey(ctx, securityGroupExternalAdoptionPrivateKey, nil)...)
	} else if isExternal && externalAdoptionPending && resp.Private != nil {
		resp.Diagnostics.Append(resp.Private.SetKey(ctx, securityGroupExternalAdoptionPrivateKey, nil)...)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func securityGroupPropertiesEqual(state, plan SecurityGroupResourceModel) bool {
	return state.Name.Equal(plan.Name) &&
		state.Associations.Equal(plan.Associations) &&
		state.AllowAllInbound.Equal(plan.AllowAllInbound) &&
		state.AllowAllOutbound.Equal(plan.AllowAllOutbound)
}

//nolint:lll,dupl // Idempotent group deletion mirrors rule deletion semantics.
func (r *SecurityGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SecurityGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.ID.ValueString()
	res, err := r.M.Core.DeleteSecurityGroupWithResponse(ctx, core.DeleteSecurityGroupJSONRequestBody{SecurityGroup: core.SecurityGroupLookup{Id: &id}})
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return
		}
		if res != nil {
			err = genericAPIError(err, res.Body)
		}
		resp.Diagnostics.AddError("Security Group Delete Error", err.Error())
		return
	}
	if res != nil && res.JSON404 != nil {
		return
	}
	if res == nil || (res.JSON200 == nil && res.StatusCode() != 204) {
		resp.Diagnostics.AddError("Security Group Delete Error", "unexpected delete response")
	}
}

//nolint:lll // Framework interface signature.
func (r *SecurityGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	resp.Diagnostics.Append(resp.Private.SetKey(ctx, securityGroupImportPrivateKey, []byte("true"))...)
}

func (r *SecurityGroupResource) read(ctx context.Context, model *SecurityGroupResourceModel) error {
	missing, err := r.readMaybeMissing(ctx, model)
	if missing && err == nil {
		return core.ErrNotFound
	}
	return err
}

//nolint:lll,gocyclo // Read normalization keeps representation decisions together.
func (r *SecurityGroupResource) readMaybeMissing(ctx context.Context, model *SecurityGroupResourceModel) (bool, error) {
	id := model.ID.ValueString()
	res, err := r.M.Core.GetSecurityGroupWithResponse(ctx, &core.GetSecurityGroupParams{SecurityGroupId: &id})
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
		return false, fmt.Errorf("unexpected response reading security group %s", id)
	}
	group := res.JSON200.SecurityGroup
	model.ID = types.StringPointerValue(group.Id)
	model.Name = types.StringPointerValue(group.Name)
	model.AllowAllInbound = types.BoolPointerValue(group.AllowAllInbound)
	model.AllowAllOutbound = types.BoolPointerValue(group.AllowAllOutbound)
	associations := []string{}
	if group.Associations != nil {
		associations = *group.Associations
	}
	value, diags := types.SetValueFrom(ctx, types.StringType, associations)
	if diags.HasError() {
		first := diags.Errors()[0]
		return false, fmt.Errorf("%s: %s", first.Summary(), first.Detail())
	}
	model.Associations = value
	if model.ExternalRules.IsNull() || model.ExternalRules.IsUnknown() {
		model.ExternalRules = types.BoolValue(false)
	}
	if model.ExternalRules.ValueBool() {
		return false, nil
	}
	rules, err := getAllSecurityGroupRules(ctx, r.M, id)
	if err != nil {
		return false, err
	}
	for _, direction := range []string{securityGroupDirectionInbound, securityGroupDirectionOutbound} {
		selected := make([]canonicalSecurityGroupRule, 0)
		for _, rule := range rules {
			if rule.Direction == direction {
				selected = append(selected, rule)
			}
		}
		legacy := legacyRepresentationForDirection(*model, direction)
		var prior []canonicalSecurityGroupRule
		if direction == securityGroupDirectionInbound {
			if legacy {
				prior, _ = listRules(ctx, model.InboundRule, direction)
			} else {
				prior, _ = listRules(ctx, model.InboundRules, direction)
			}
		} else {
			if legacy {
				prior, _ = listRules(ctx, model.OutboundRule, direction)
			} else {
				prior, _ = listRules(ctx, model.OutboundRules, direction)
			}
		}
		priorByID := make(map[string]string, len(prior))
		for _, old := range prior {
			priorByID[old.ID] = old.Protocol
		}
		for i := range selected {
			if oldProtocol := priorByID[selected[i].ID]; strings.EqualFold(oldProtocol, selected[i].Protocol) && oldProtocol != "" {
				selected[i].Protocol = oldProtocol
			}
		}
		selected = preserveSecurityGroupRuleStateOrder(prior, selected)
		if err := setSecurityGroupRules(ctx, model, direction, legacy, selected); err != nil {
			return false, err
		}
	}
	return false, nil
}

func preserveSecurityGroupRuleStateOrder(
	prior, remote []canonicalSecurityGroupRule,
) []canonicalSecurityGroupRule {
	remoteByID := make(map[string]canonicalSecurityGroupRule, len(remote))
	for _, rule := range remote {
		remoteByID[rule.ID] = rule
	}
	ordered := make([]canonicalSecurityGroupRule, 0, len(remote))
	for _, priorRule := range prior {
		if rule, ok := remoteByID[priorRule.ID]; ok {
			ordered = append(ordered, rule)
			delete(remoteByID, priorRule.ID)
		}
	}
	for _, rule := range remote {
		if _, ok := remoteByID[rule.ID]; ok {
			ordered = append(ordered, rule)
			delete(remoteByID, rule.ID)
		}
	}
	return ordered
}

//nolint:lll,gocritic // Direction and representation shape the public state path.
func setSecurityGroupRules(ctx context.Context, model *SecurityGroupResourceModel, direction string, legacy bool, rules []canonicalSecurityGroupRule) error {
	value, diags := securityGroupRulesListValue(ctx, rules)
	if diags.HasError() {
		first := diags.Errors()[0]
		return fmt.Errorf("%s: %s", first.Summary(), first.Detail())
	}
	null := types.ListNull(securityGroupRuleObjectType())
	if direction == securityGroupDirectionInbound {
		if legacy {
			model.InboundRule, model.InboundRules = value, null
		} else {
			model.InboundRules, model.InboundRule = value, null
		}
	} else if legacy {
		model.OutboundRule, model.OutboundRules = value, null
	} else {
		model.OutboundRules, model.OutboundRule = value, null
	}
	return nil
}

//nolint:lll // API request projection remains explicit.
func securityGroupRuleAPIArguments(rule canonicalSecurityGroupRule) core.SecurityGroupRuleArguments {
	direction := core.SecurityGroupRuleDirectionEnum(strings.ToLower(rule.Direction))
	protocol := core.SecurityGroupRuleProtocolEnum(strings.ToUpper(rule.Protocol))
	targets := append([]string(nil), rule.Targets...)
	return core.SecurityGroupRuleArguments{Direction: &direction, Protocol: &protocol, Ports: &rule.Ports, Targets: &targets, Notes: &rule.Notes}
}

type securityGroupRuleReconciliation struct {
	Desired       []canonicalSecurityGroupRule
	CreateIndexes []int
	UpdateIndexes []int
	Delete        []canonicalSecurityGroupRule
}

func classifySecurityGroupRuleReconciliation(
	prior, desired []canonicalSecurityGroupRule,
) securityGroupRuleReconciliation {
	desired = transferSecurityGroupRuleIDs(prior, desired, false)
	result := securityGroupRuleReconciliation{Desired: desired}
	priorByID := make(map[string]canonicalSecurityGroupRule, len(prior))
	for _, rule := range prior {
		if rule.ID != "" {
			priorByID[rule.ID] = rule
		}
	}
	kept := make(map[string]bool, len(desired))
	for i, rule := range desired {
		if rule.ID == "" {
			result.CreateIndexes = append(result.CreateIndexes, i)
			continue
		}
		if old, ok := priorByID[rule.ID]; ok && old.fingerprint() != rule.fingerprint() {
			result.UpdateIndexes = append(result.UpdateIndexes, i)
		}
		kept[rule.ID] = true
	}
	for _, rule := range prior {
		if rule.ID != "" && !kept[rule.ID] {
			result.Delete = append(result.Delete, rule)
		}
	}

	return result
}

//nolint:lll,gocyclo // Reconciliation order is material and intentionally linear.
func (r *SecurityGroupResource) reconcileRules(ctx context.Context, groupID string, prior, desired []canonicalSecurityGroupRule) ([]canonicalSecurityGroupRule, error) {
	reconciliation := classifySecurityGroupRuleReconciliation(prior, desired)
	desired = reconciliation.Desired
	create := make(map[int]bool, len(reconciliation.CreateIndexes))
	for _, index := range reconciliation.CreateIndexes {
		create[index] = true
	}
	update := make(map[int]bool, len(reconciliation.UpdateIndexes))
	for _, index := range reconciliation.UpdateIndexes {
		update[index] = true
	}
	for i := range desired {
		if create[i] {
			res, err := r.M.Core.PostSecurityGroupRulesWithResponse(ctx, core.PostSecurityGroupRulesJSONRequestBody{SecurityGroup: core.SecurityGroupLookup{Id: &groupID}, Properties: securityGroupRuleAPIArguments(desired[i])})
			if err != nil {
				if res != nil {
					err = genericAPIError(err, res.Body)
				}
				return nil, err
			}
			if res == nil || res.JSON200 == nil || res.JSON200.SecurityGroupRule.Id == nil {
				return nil, fmt.Errorf("unexpected rule create response")
			}
			desired[i].ID = *res.JSON200.SecurityGroupRule.Id
			desired[i].Protocol = strings.ToUpper(desired[i].Protocol)
		} else if update[i] {
			id := desired[i].ID
			res, err := r.M.Core.PatchSecurityGroupsRulesSecurityGroupRuleWithResponse(ctx, core.PatchSecurityGroupsRulesSecurityGroupRuleJSONRequestBody{SecurityGroupRule: core.SecurityGroupRuleLookup{Id: &id}, Properties: securityGroupRuleAPIArguments(desired[i])})
			if err != nil {
				if res != nil {
					err = genericAPIError(err, res.Body)
				}
				return nil, err
			}
			if res == nil || res.JSON200 == nil {
				return nil, fmt.Errorf("unexpected rule update response")
			}
		}
	}
	for _, rule := range reconciliation.Delete {
		id := rule.ID
		res, err := r.M.Core.DeleteSecurityGroupsRulesSecurityGroupRuleWithResponse(ctx, core.DeleteSecurityGroupsRulesSecurityGroupRuleJSONRequestBody{SecurityGroupRule: core.SecurityGroupRuleLookup{Id: &id}})
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				continue
			}
			if res != nil {
				err = genericAPIError(err, res.Body)
			}
			return nil, err
		}
		if res != nil && res.JSON404 != nil {
			continue
		}
		if res == nil || (res.JSON200 == nil && res.StatusCode() != 204) {
			return nil, fmt.Errorf("unexpected rule delete response")
		}
	}
	return desired, nil
}
