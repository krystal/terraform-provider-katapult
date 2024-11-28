package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// NullToEmptySetPlanModifier ensures that a set attribute which is removed from
// a resource definition outright (rather than set to a empty set) while it has
// values in the state, is planned as a empty to set trigger an update to remove
// all existing values.
func NullToEmptySetPlanModifier() planmodifier.Set {
	return nullToEmptySetPlanModifier{}
}

// nullToEmptySetPlanModifier implements the plan modifier.
type nullToEmptySetPlanModifier struct{}

// Description returns a human-readable description of the plan modifier.
func (m nullToEmptySetPlanModifier) Description(_ context.Context) string {
	return "Modify removed set attributes to be planned as a empty set."
}

// MarkdownDescription returns a markdown description of the plan modifier.
func (m nullToEmptySetPlanModifier) MarkdownDescription(
	_ context.Context,
) string {
	return "Modify removed set attributes to be planned as a empty set."
}

// PlanModifySet implements the plan modification logic.
func (m nullToEmptySetPlanModifier) PlanModifySet(
	ctx context.Context,
	req planmodifier.SetRequest,
	resp *planmodifier.SetResponse,
) {
	// When plan is unknown, the resource does not yet exist, so we should
	// set it to a null set to avoid unknown type errors.
	if req.PlanValue.IsUnknown() {
		resp.PlanValue = types.SetNull(req.PlanValue.ElementType(ctx))
		return
	}

	// When plan has elements but the config is null, then the attribute has
	// been completely removed from the configuration. To ensure the plan knows
	// to remove all existing values, we must set the plan to a empty set.
	if len(req.PlanValue.Elements()) > 0 && req.ConfigValue.IsNull() {
		resp.PlanValue = types.SetValueMust(
			req.PlanValue.ElementType(ctx), []attr.Value{},
		)
		return
	}
}
