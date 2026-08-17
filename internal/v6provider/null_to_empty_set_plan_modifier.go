package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// NullToEmptySetPlanModifier ensures that a set attribute which is removed from
// a resource definition outright (rather than set to an empty set) while it has
// values in the state, is planned as an empty set to trigger an update to remove
// all existing values.
func NullToEmptySetPlanModifier() planmodifier.Set {
	return nullToEmptySetPlanModifier{}
}

// nullToEmptySetPlanModifier implements the plan modifier.
type nullToEmptySetPlanModifier struct{}

// Description returns a human-readable description of the plan modifier.
func (m nullToEmptySetPlanModifier) Description(_ context.Context) string {
	return "Modify removed set attributes to be planned as an empty set."
}

// MarkdownDescription returns a markdown description of the plan modifier.
func (m nullToEmptySetPlanModifier) MarkdownDescription(
	_ context.Context,
) string {
	return "Modify removed set attributes to be planned as an empty set."
}

// PlanModifySet implements the plan modification logic.
func (m nullToEmptySetPlanModifier) PlanModifySet(
	ctx context.Context,
	req planmodifier.SetRequest,
	resp *planmodifier.SetResponse,
) {
	// Preserve a null value from existing state. SDKv2 resources can store an
	// omitted optional+computed set as null, which is already equivalent to an
	// empty remote collection and should not create a migration-only diff.
	if req.ConfigValue.IsNull() && req.StateValue.IsNull() {
		if !req.State.Raw.IsNull() {
			resp.PlanValue = req.StateValue
		}
		return
	}

	// The framework marks unconfigured Optional+Computed attributes unknown on
	// every update. This attribute's contract treats omission as an empty set,
	// so resolve that framework-generated unknown instead of exposing unrelated
	// plan noise.
	if req.PlanValue.IsUnknown() {
		if req.ConfigValue.IsNull() {
			resp.PlanValue = types.SetValueMust(
				req.PlanValue.ElementType(ctx), []attr.Value{},
			)
		}
		return
	}

	// When plan is null (attribute not set in config), normalize it to an empty
	// set so the provider can remove any existing remote values.
	if req.PlanValue.IsNull() {
		resp.PlanValue = types.SetValueMust(
			req.PlanValue.ElementType(ctx), []attr.Value{},
		)
		return
	}

	// When plan has elements but the config is null, then the attribute has
	// been completely removed from the configuration. To ensure the plan knows
	// to remove all existing values, we must set the plan to an empty set.
	if len(req.PlanValue.Elements()) > 0 && req.ConfigValue.IsNull() {
		resp.PlanValue = types.SetValueMust(
			req.PlanValue.ElementType(ctx), []attr.Value{},
		)
		return
	}
}
