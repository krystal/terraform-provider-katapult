package v6provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
)

func Test_stringValidatorNotEmpty(t *testing.T) {
	tests := []struct {
		name      string
		value     types.String
		wantError bool
	}{
		{
			name:  "unknown",
			value: types.StringUnknown(),
		},
		{
			name:  "null",
			value: types.StringNull(),
		},
		{
			name:  "valid",
			value: types.StringValue("ok"),
		},
		{
			name:      "empty",
			value:     types.StringValue(""),
			wantError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			request := validator.StringRequest{ConfigValue: tt.value}
			response := validator.StringResponse{}

			stringValidatorNotEmpty().ValidateString(ctx, request, &response)

			assert.Equal(t, response.Diagnostics.HasError(), tt.wantError)
		})
	}
}
