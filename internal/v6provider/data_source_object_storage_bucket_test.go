package v6provider

import (
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceObjectStorageBucket_minimal(t *testing.T) {
	tt := newTestTools(t)
	name := strings.ToLower(tt.ResourceName())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultObjectStorageBucketDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_object_storage_bucket" "main" {
					  name   = "%s"
					  region = "uk-lon-1"
					}

					data "katapult_object_storage_bucket" "main" {
					  name   = katapult_object_storage_bucket.main.name
					  region = katapult_object_storage_bucket.main.region
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultDataSourceObjectStorageBucketAttrs(
						tt, "data.katapult_object_storage_bucket.main",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_object_storage_bucket.main", "name",
						"katapult_object_storage_bucket.main", "name",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_object_storage_bucket.main", "region",
						"katapult_object_storage_bucket.main", "region",
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

func TestAccKatapultDataSourceObjectStorageBucket_not_found(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					data "katapult_object_storage_bucket" "main" {
					  name   = "this-bucket-does-not-exist"
					  region = "uk-lon-1"
					}`,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta("resource not found"),
				),
			},
		},
	})
}

func testAccCheckKatapultDataSourceObjectStorageBucketAttrs(
	tt *testTools,
	res string,
) resource.TestCheckFunc {
	return testAccCheckKatapultObjectStorageBucketAttrs(tt, res)
}
