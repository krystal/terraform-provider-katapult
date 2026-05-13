package v6provider

import (
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jimeh/undent"
)

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
					  name                      = "%s"
					  object_storage_account_id = data.katapult_object_storage_account.main.id
					}

					data "katapult_object_storage_bucket" "main" {
					  name                      = katapult_object_storage_bucket.main.name
					  object_storage_account_id = katapult_object_storage_bucket.main.object_storage_account_id
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
						"object_storage_account_id",
						"katapult_object_storage_bucket.main",
						"object_storage_account_id",
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
					  name                      = "this-bucket-does-not-exist"
					  object_storage_account_id = "%s"
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
