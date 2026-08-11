package v6provider

import (
	"context"
	"testing"

	frameworkdatasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	frameworkvalidator "github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"
)

func TestObjectStorageRegionSchema(t *testing.T) {
	ctx := context.Background()

	t.Run("account resource", func(t *testing.T) {
		var resp frameworkresource.SchemaResponse
		(&ObjectStorageAccountResource{}).Schema(
			ctx, frameworkresource.SchemaRequest{}, &resp,
		)

		require.NotContains(t, resp.Schema.Attributes, "id")
		requireRequiredResourceRegion(ctx, t, resp.Schema.Attributes)
	})

	t.Run("account data source", func(t *testing.T) {
		var resp frameworkdatasource.SchemaResponse
		(&ObjectStorageAccountDataSource{}).Schema(
			ctx, frameworkdatasource.SchemaRequest{}, &resp,
		)

		require.NotContains(t, resp.Schema.Attributes, "id")
		requireRequiredDataSourceRegion(ctx, t, resp.Schema.Attributes)
	})

	t.Run("bucket resource", func(t *testing.T) {
		var resp frameworkresource.SchemaResponse
		(&ObjectStorageBucketResource{}).Schema(
			ctx, frameworkresource.SchemaRequest{}, &resp,
		)

		require.NotContains(t, resp.Schema.Attributes, "object_storage_account_id")
		requireRequiredResourceRegion(ctx, t, resp.Schema.Attributes)
	})

	t.Run("bucket data source", func(t *testing.T) {
		var resp frameworkdatasource.SchemaResponse
		(&ObjectStorageBucketDataSource{}).Schema(
			ctx, frameworkdatasource.SchemaRequest{}, &resp,
		)

		require.NotContains(t, resp.Schema.Attributes, "object_storage_account_id")
		requireRequiredDataSourceRegion(ctx, t, resp.Schema.Attributes)
	})

	t.Run("access key resource", func(t *testing.T) {
		var resp frameworkresource.SchemaResponse
		(&ObjectStorageAccessKeyResource{}).Schema(
			ctx, frameworkresource.SchemaRequest{}, &resp,
		)

		require.NotContains(t, resp.Schema.Attributes, "object_storage_account_id")
		requireRequiredResourceRegion(ctx, t, resp.Schema.Attributes)
	})
}

func requireRequiredResourceRegion(
	ctx context.Context,
	t *testing.T,
	attributes map[string]resourceschema.Attribute,
) {
	t.Helper()

	attribute, ok := attributes[objectStorageRegionAttributeName].(resourceschema.StringAttribute)
	require.True(t, ok)
	require.True(t, attribute.Required)
	require.False(t, attribute.Optional)
	require.False(t, attribute.Computed)
	require.NotEmpty(t, attribute.PlanModifiers)
	requireRegionValidatorsAcceptArbitraryValue(ctx, t, attribute.Validators)
}

func requireRequiredDataSourceRegion(
	ctx context.Context,
	t *testing.T,
	attributes map[string]datasourceschema.Attribute,
) {
	t.Helper()

	attribute, ok := attributes[objectStorageRegionAttributeName].(datasourceschema.StringAttribute)
	require.True(t, ok)
	require.True(t, attribute.Required)
	require.False(t, attribute.Optional)
	require.False(t, attribute.Computed)
	requireRegionValidatorsAcceptArbitraryValue(ctx, t, attribute.Validators)
}

func requireRegionValidatorsAcceptArbitraryValue(
	ctx context.Context,
	t *testing.T,
	validators []frameworkvalidator.String,
) {
	t.Helper()
	require.NotEmpty(t, validators)

	for _, regionValidator := range validators {
		request := frameworkvalidator.StringRequest{
			ConfigValue: types.StringValue("future-region"),
		}
		response := &frameworkvalidator.StringResponse{}
		regionValidator.ValidateString(ctx, request, response)
		require.False(t, response.Diagnostics.HasError())
	}
}
