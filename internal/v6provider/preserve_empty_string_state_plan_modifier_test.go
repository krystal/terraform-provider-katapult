package v6provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"
)

func TestPreserveEmptyStringStateForNullConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		config     types.String
		plan       types.String
		state      types.String
		wantResult types.String
	}{
		{
			name:       "legacy empty state",
			config:     types.StringNull(),
			plan:       types.StringUnknown(),
			state:      types.StringValue(""),
			wantResult: types.StringValue(""),
		},
		{
			name:       "null state remains known",
			config:     types.StringNull(),
			plan:       types.StringUnknown(),
			state:      types.StringNull(),
			wantResult: types.StringNull(),
		},
		{
			name:       "configured value",
			config:     types.StringValue("configured"),
			plan:       types.StringValue("configured"),
			state:      types.StringValue(""),
			wantResult: types.StringValue("configured"),
		},
		{
			name:       "removed non-empty value",
			config:     types.StringNull(),
			plan:       types.StringValue("existing"),
			state:      types.StringValue("existing"),
			wantResult: types.StringNull(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp := &planmodifier.StringResponse{PlanValue: tt.plan}
			PreserveEmptyStringStateForNullConfig().PlanModifyString(
				context.Background(),
				planmodifier.StringRequest{
					ConfigValue: tt.config,
					PlanValue:   tt.plan,
					StateValue:  tt.state,
				},
				resp,
			)

			require.True(t, resp.PlanValue.Equal(tt.wantResult))
		})
	}
}
