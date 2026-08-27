package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.ConfigValidator = nonEmptySelectorConfigValidator{}

type nonEmptySelectorConfigValidator struct{}

func (nonEmptySelectorConfigValidator) Description(context.Context) string {
	return "at least one of id,permalink must be a non-empty string"
}

func (v nonEmptySelectorConfigValidator) MarkdownDescription(
	ctx context.Context,
) string {
	return v.Description(ctx)
}

func (v nonEmptySelectorConfigValidator) ValidateDataSource(
	ctx context.Context,
	req datasource.ValidateConfigRequest,
	resp *datasource.ValidateConfigResponse,
) {
	var id, permalink types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("id"), &id)...)
	resp.Diagnostics.Append(
		req.Config.GetAttribute(ctx, path.Root("permalink"), &permalink)...,
	)
	if resp.Diagnostics.HasError() {
		return
	}

	if configuredStringSelector(id) || configuredStringSelector(permalink) {
		return
	}
	if id.IsUnknown() || permalink.IsUnknown() {
		return
	}

	resp.Diagnostics.AddError("Missing Attribute Configuration", v.Description(ctx))
}

func configuredStringSelector(value types.String) bool {
	return !value.IsNull() && !value.IsUnknown() && value.ValueString() != ""
}

func selectedStringSelector(
	id types.String,
	permalink types.String,
) (string, string) {
	if configuredStringSelector(id) {
		return "id", id.ValueString()
	}

	return "permalink", permalink.ValueString()
}
