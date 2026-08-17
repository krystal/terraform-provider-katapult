package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// PreserveEmptyStringStateForNullConfig avoids a migration-only diff when an
// SDKv2 resource stored an omitted optional string as an empty string.
func PreserveEmptyStringStateForNullConfig() planmodifier.String {
	return preserveEmptyStringStateForNullConfigModifier{}
}

type preserveEmptyStringStateForNullConfigModifier struct{}

func (preserveEmptyStringStateForNullConfigModifier) Description(
	_ context.Context,
) string {
	return "Preserve an empty-string state value when configuration is null."
}

func (m preserveEmptyStringStateForNullConfigModifier) MarkdownDescription(
	ctx context.Context,
) string {
	return m.Description(ctx)
}

func (preserveEmptyStringStateForNullConfigModifier) PlanModifyString(
	_ context.Context,
	req planmodifier.StringRequest,
	resp *planmodifier.StringResponse,
) {
	if !req.ConfigValue.IsNull() || req.StateValue.IsUnknown() {
		return
	}
	if req.StateValue.IsNull() {
		resp.PlanValue = req.StateValue
		return
	}

	if req.StateValue.ValueString() == "" {
		resp.PlanValue = req.StateValue
		return
	}

	resp.PlanValue = types.StringNull()
}
