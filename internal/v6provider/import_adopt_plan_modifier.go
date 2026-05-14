package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
)

// Import-adopt plan modifiers let attributes that are configured by the user
// at create time, but cannot be read back from the API, survive
// `terraform import` without forcing replacement.
//
// On the first plan after import, the prior state value is null (vmRead
// could not populate it) while the config value is set. The framework would
// otherwise treat this as a change on a RequiresReplace attribute and mark
// the resource for replacement. These modifiers detect that exact shape
// (resource exists in state, attribute is null, config is set) and adopt
// the config value into the plan without triggering replacement.

func ImportAdoptStringPlanModifier() planmodifier.String {
	return importAdoptStringPlanModifier{}
}

type importAdoptStringPlanModifier struct{}

func (importAdoptStringPlanModifier) Description(_ context.Context) string {
	return "Adopt the config value on the first plan after import without " +
		"triggering replacement."
}

func (m importAdoptStringPlanModifier) MarkdownDescription(
	ctx context.Context,
) string {
	return m.Description(ctx)
}

func (importAdoptStringPlanModifier) PlanModifyString(
	_ context.Context,
	req planmodifier.StringRequest,
	resp *planmodifier.StringResponse,
) {
	if req.State.Raw.IsNull() {
		return
	}
	if !req.StateValue.IsNull() {
		return
	}
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	resp.PlanValue = req.ConfigValue
	resp.RequiresReplace = false
}

func ImportAdoptMapPlanModifier() planmodifier.Map {
	return importAdoptMapPlanModifier{}
}

type importAdoptMapPlanModifier struct{}

func (importAdoptMapPlanModifier) Description(_ context.Context) string {
	return "Adopt the config value on the first plan after import without " +
		"triggering replacement."
}

func (m importAdoptMapPlanModifier) MarkdownDescription(
	ctx context.Context,
) string {
	return m.Description(ctx)
}

func (importAdoptMapPlanModifier) PlanModifyMap(
	_ context.Context,
	req planmodifier.MapRequest,
	resp *planmodifier.MapResponse,
) {
	if req.State.Raw.IsNull() {
		return
	}
	if !req.StateValue.IsNull() {
		return
	}
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	resp.PlanValue = req.ConfigValue
	resp.RequiresReplace = false
}

func ImportAdoptListPlanModifier() planmodifier.List {
	return importAdoptListPlanModifier{}
}

type importAdoptListPlanModifier struct{}

func (importAdoptListPlanModifier) Description(_ context.Context) string {
	return "Adopt the config value on the first plan after import without " +
		"triggering replacement."
}

func (m importAdoptListPlanModifier) MarkdownDescription(
	ctx context.Context,
) string {
	return m.Description(ctx)
}

func (importAdoptListPlanModifier) PlanModifyList(
	_ context.Context,
	req planmodifier.ListRequest,
	resp *planmodifier.ListResponse,
) {
	if req.State.Raw.IsNull() {
		return
	}
	if !req.StateValue.IsNull() {
		return
	}
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	resp.PlanValue = req.ConfigValue
	resp.RequiresReplace = false
}
