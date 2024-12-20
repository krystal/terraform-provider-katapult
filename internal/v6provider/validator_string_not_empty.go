package v6provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/helpers/validatordiag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

var _ validator.String = stringNotEmptyValidator{}

// stringLenAtLeastValidator validates that a string is not empty.
type stringNotEmptyValidator struct{}

// Description describes the validation in plain text formatting.
func (v stringNotEmptyValidator) Description(_ context.Context) string {
	return "string cannot be empty"
}

// MarkdownDescription describes the validation in Markdown formatting.
func (v stringNotEmptyValidator) MarkdownDescription(
	ctx context.Context,
) string {
	return v.Description(ctx)
}

// Validate performs the validation.
func (v stringNotEmptyValidator) ValidateString(
	ctx context.Context,
	request validator.StringRequest,
	response *validator.StringResponse,
) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	value := request.ConfigValue.ValueString()

	if value != "" {
		return
	}

	response.Diagnostics.Append(
		validatordiag.InvalidAttributeValueLengthDiagnostic(
			request.Path,
			v.Description(ctx),
			value,
		),
	)
}

// stringValidatorNotEmpty returns a validator that ensures a string is not
// empty.
func stringValidatorNotEmpty() validator.String {
	return stringNotEmptyValidator{}
}
