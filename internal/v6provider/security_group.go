package v6provider

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

type SecurityGroupRuleModel struct {
	ID        types.String               `tfsdk:"id"`
	Direction types.String               `tfsdk:"direction"`
	Protocol  caseInsensitiveStringValue `tfsdk:"protocol"`
	Ports     types.String               `tfsdk:"ports"`
	Targets   types.Set                  `tfsdk:"targets"`
	Notes     types.String               `tfsdk:"notes"`
}

const (
	securityGroupAssociationsAttribute = "associations"
	securityGroupDirectionInbound      = "inbound"
	securityGroupDirectionOutbound     = "outbound"
)

type canonicalSecurityGroupRule struct {
	ID        string
	Direction string
	Protocol  string
	Ports     string
	Targets   []string
	Notes     string
}

func (r canonicalSecurityGroupRule) fingerprint() string {
	targets := append([]string(nil), r.Targets...)
	sort.Strings(targets)

	return strings.Join([]string{
		strings.ToLower(r.Direction), strings.ToUpper(r.Protocol), r.Ports,
		strings.Join(targets, "\x00"), r.Notes,
	}, "\x01")
}

func canonicalRulesFromList(
	ctx context.Context,
	value types.List,
	direction string,
) ([]canonicalSecurityGroupRule, diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return nil, nil
	}

	var models []SecurityGroupRuleModel
	diags := value.ElementsAs(ctx, &models, false)
	if diags.HasError() {
		return nil, diags
	}

	rules := make([]canonicalSecurityGroupRule, 0, len(models))
	for _, model := range models {
		var targets []string
		diags.Append(model.Targets.ElementsAs(ctx, &targets, false)...)
		if diags.HasError() {
			return nil, diags
		}
		ruleDirection := direction
		if !model.Direction.IsNull() && !model.Direction.IsUnknown() {
			ruleDirection = strings.ToLower(model.Direction.ValueString())
		}
		rules = append(rules, canonicalSecurityGroupRule{
			ID:        model.ID.ValueString(),
			Direction: ruleDirection,
			Protocol:  model.Protocol.ValueString(),
			Ports:     model.Ports.ValueString(),
			Targets:   targets,
			Notes:     model.Notes.ValueString(),
		})
	}

	return rules, diags
}

func securityGroupRuleModels(
	ctx context.Context,
	rules []canonicalSecurityGroupRule,
	unknownEmptyIDs bool,
) ([]SecurityGroupRuleModel, diag.Diagnostics) {
	models := make([]SecurityGroupRuleModel, 0, len(rules))
	var diags diag.Diagnostics
	for _, rule := range rules {
		targets, targetDiags := types.SetValueFrom(ctx, types.StringType, rule.Targets)
		diags.Append(targetDiags...)
		id := types.StringValue(rule.ID)
		if unknownEmptyIDs && rule.ID == "" {
			id = types.StringUnknown()
		}
		models = append(models, SecurityGroupRuleModel{
			ID:        id,
			Direction: types.StringValue(strings.ToLower(rule.Direction)),
			Protocol:  caseInsensitiveStringValueOf(rule.Protocol),
			Ports:     types.StringValue(rule.Ports),
			Targets:   targets,
			Notes:     types.StringValue(rule.Notes),
		})
	}

	return models, diags
}

func securityGroupRulesListValue(
	ctx context.Context,
	rules []canonicalSecurityGroupRule,
) (types.List, diag.Diagnostics) {
	models, diags := securityGroupRuleModels(ctx, rules, false)
	if diags.HasError() {
		return types.ListNull(securityGroupRuleObjectType()), diags
	}
	value, valueDiags := types.ListValueFrom(ctx, securityGroupRuleObjectType(), models)
	diags.Append(valueDiags...)

	return value, diags
}

func securityGroupRulesPlanListValue(
	ctx context.Context,
	rules []canonicalSecurityGroupRule,
) (types.List, diag.Diagnostics) {
	models, diags := securityGroupRuleModels(ctx, rules, true)
	if diags.HasError() {
		return types.ListNull(securityGroupRuleObjectType()), diags
	}
	value, valueDiags := types.ListValueFrom(ctx, securityGroupRuleObjectType(), models)
	diags.Append(valueDiags...)

	return value, diags
}

func securityGroupRuleBlockPlanValue(
	value types.List,
) (types.List, diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return value, nil
	}
	elements := make([]attr.Value, 0, len(value.Elements()))
	var diags diag.Diagnostics
	for _, element := range value.Elements() {
		object, ok := element.(types.Object)
		if !ok || object.IsNull() || object.IsUnknown() {
			elements = append(elements, element)
			continue
		}
		attributes := object.Attributes()
		attributes["id"] = types.StringUnknown()
		attributes["direction"] = types.StringUnknown()
		planned, objectDiags := types.ObjectValue(
			securityGroupRuleObjectType().AttrTypes,
			attributes,
		)
		diags.Append(objectDiags...)
		elements = append(elements, planned)
	}
	planned, valueDiags := types.ListValue(securityGroupRuleObjectType(), elements)
	diags.Append(valueDiags...)
	return planned, diags
}

func securityGroupRuleObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"id": types.StringType, "direction": types.StringType,
		"protocol": caseInsensitiveStringType{}, "ports": types.StringType,
		"targets": types.SetType{ElemType: types.StringType},
		"notes":   types.StringType,
	}}
}

// transferSecurityGroupRuleIDs deterministically pairs equivalent rules. ID
// matches take priority, followed by semantic matches in prior-state order.
func transferSecurityGroupRuleIDs(
	prior, planned []canonicalSecurityGroupRule,
) []canonicalSecurityGroupRule {
	result := append([]canonicalSecurityGroupRule(nil), planned...)
	used := make([]bool, len(prior))

	for i := range result {
		if result[i].ID == "" {
			continue
		}
		for j := range prior {
			if !used[j] && prior[j].ID == result[i].ID {
				used[j] = true
				break
			}
		}
	}
	for i := range result {
		if result[i].ID != "" {
			continue
		}
		fingerprint := result[i].fingerprint()
		for j := range prior {
			if used[j] || prior[j].fingerprint() != fingerprint {
				continue
			}
			result[i].ID = prior[j].ID
			used[j] = true
			break
		}
	}

	return result
}

func nullableString(value interface {
	IsSpecified() bool
	IsNull() bool
	Get() (string, error)
},
) string {
	if !value.IsSpecified() || value.IsNull() {
		return ""
	}
	result, err := value.Get()
	if err != nil {
		return ""
	}

	return result
}

func canonicalRuleFromListResult(
	rule core.GetSecurityGroupRules200ResponseSecurityGroupRules,
) canonicalSecurityGroupRule {
	result := canonicalSecurityGroupRule{}
	if rule.Id != nil {
		result.ID = *rule.Id
	}
	if rule.Direction != nil {
		result.Direction = strings.ToLower(string(*rule.Direction))
	}
	if rule.Protocol != nil {
		result.Protocol = strings.ToUpper(string(*rule.Protocol))
	}
	result.Ports = nullableString(rule.Ports)
	result.Notes = nullableString(rule.Notes)
	if rule.Targets != nil {
		result.Targets = append([]string(nil), (*rule.Targets)...)
	}

	return result
}

func getAllSecurityGroupRules(
	ctx context.Context,
	m *Meta,
	securityGroupID string,
) ([]canonicalSecurityGroupRule, error) {
	rules := []canonicalSecurityGroupRule{}
	totalPages := 1
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		res, err := m.Core.GetSecurityGroupRulesWithResponse(ctx,
			&core.GetSecurityGroupRulesParams{
				SecurityGroupId: &securityGroupID,
				Page:            &pageNum,
			})
		if err != nil {
			if res != nil {
				err = genericAPIError(err, res.Body)
			}
			return nil, err
		}
		if res == nil || res.JSON200 == nil {
			return nil, fmt.Errorf("unexpected response listing rules for security group %s", securityGroupID)
		}
		for _, rule := range res.JSON200.SecurityGroupRules {
			rules = append(rules, canonicalRuleFromListResult(rule))
		}
		totalPages, _ = res.JSON200.Pagination.TotalPages.Get()
	}

	return rules, nil
}
