package v6provider

import (
	"context"
	"reflect"
	"regexp"
	"strings"
	"testing"

	frameworkdatasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jimeh/undent"
	"github.com/stretchr/testify/require"
)

func TestObjectStorageBucketDataSourceSchemaMatchesSharedModel(t *testing.T) {
	ctx := context.Background()
	var resp frameworkdatasource.SchemaResponse
	(&ObjectStorageBucketDataSource{}).Schema(
		ctx, frameworkdatasource.SchemaRequest{}, &resp,
	)
	require.False(t, resp.Diagnostics.HasError())

	modelAttributes := make(map[string]struct{})
	modelType := reflect.TypeOf(ObjectStorageBucketResourceModel{})
	for i := range modelType.NumField() {
		field := modelType.Field(i)
		tag, ok := field.Tag.Lookup("tfsdk")
		require.Truef(t, ok, "model field %s has no tfsdk tag", field.Name)
		name := strings.Split(tag, ",")[0]
		require.NotEmptyf(t, name, "model field %s has an empty tfsdk tag", field.Name)
		modelAttributes[name] = struct{}{}
	}

	schemaAttributes := make(map[string]struct{}, len(resp.Schema.Attributes))
	for name := range resp.Schema.Attributes {
		schemaAttributes[name] = struct{}{}
	}

	require.Equal(t, modelAttributes, schemaAttributes)
}

func accDataSourceObjectStorageBucketMinimal(t *testing.T) {
	tt := newTestTools(t)
	name := strings.ToLower(tt.ResourceName())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultObjectStorageBucketDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: objectStorageAccountDataBlock + undent.Stringf(`
					resource "katapult_object_storage_bucket" "main" {
					  name = "%s"
					  region = data.katapult_object_storage_account.main.region
					}

					data "katapult_object_storage_bucket" "main" {
					  name = katapult_object_storage_bucket.main.name
					  region = katapult_object_storage_bucket.main.region
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultObjectStorageBucketAttrs(
						tt, "data.katapult_object_storage_bucket.main",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_object_storage_bucket.main", "name",
						"katapult_object_storage_bucket.main", "name",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_object_storage_bucket.main",
						"region",
						"katapult_object_storage_bucket.main",
						"region",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_object_storage_bucket.main", "public_url",
						"katapult_object_storage_bucket.main", "public_url",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_object_storage_bucket.main", "serve_static_site",
						"katapult_object_storage_bucket.main", "serve_static_site",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_object_storage_bucket.main", "all_keys_read",
						"katapult_object_storage_bucket.main", "all_keys_read",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_object_storage_bucket.main", "all_keys_write",
						"katapult_object_storage_bucket.main", "all_keys_write",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_object_storage_bucket.main", "public_list",
						"katapult_object_storage_bucket.main", "public_list",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_object_storage_bucket.main", "public_read",
						"katapult_object_storage_bucket.main", "public_read",
					),
				),
			},
		},
	})
}

func accDataSourceObjectStorageBucketNotFound(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					data "katapult_object_storage_bucket" "main" {
					  name = "this-bucket-does-not-exist"
					  region = "%s"
					}`,
					objectStorageAccTestRegion,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta("resource not found"),
				),
			},
		},
	})
}
