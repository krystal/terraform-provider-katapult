package v6provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"
)

func TestNullToEmptySetPlanModifier(t *testing.T) {
	t.Parallel()

	empty := types.SetValueMust(types.StringType, []attr.Value{})
	populated := types.SetValueMust(types.StringType, []attr.Value{
		types.StringValue("existing"),
	})

	tests := []struct {
		name       string
		config     types.Set
		plan       types.Set
		state      types.Set
		wantResult types.Set
	}{
		{
			name:       "legacy null state",
			config:     types.SetNull(types.StringType),
			plan:       types.SetNull(types.StringType),
			state:      types.SetNull(types.StringType),
			wantResult: types.SetNull(types.StringType),
		},
		{
			name:       "unknown plan",
			config:     types.SetNull(types.StringType),
			plan:       types.SetUnknown(types.StringType),
			state:      populated,
			wantResult: types.SetUnknown(types.StringType),
		},
		{
			name:       "removed populated state",
			config:     types.SetNull(types.StringType),
			plan:       populated,
			state:      populated,
			wantResult: empty,
		},
		{
			name:       "removed empty state",
			config:     types.SetNull(types.StringType),
			plan:       types.SetNull(types.StringType),
			state:      empty,
			wantResult: empty,
		},
		{
			name:       "configured values",
			config:     populated,
			plan:       populated,
			state:      empty,
			wantResult: populated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp := &planmodifier.SetResponse{PlanValue: tt.plan}
			NullToEmptySetPlanModifier().PlanModifySet(
				context.Background(),
				planmodifier.SetRequest{
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
