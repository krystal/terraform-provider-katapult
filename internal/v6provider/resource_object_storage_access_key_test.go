package v6provider

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/next/core"
)

// objectStorageAccountDataBlock is the canonical reference to the shared
// account from inside subtest configs.
const objectStorageAccountDataBlock = `
data "katapult_object_storage_account" "main" {
  region = "uk-lon-1"
}
`

// accObjectStorageAccessKeyMinimal exercises the minimal create + import
// path for an access key against the shared object storage account.
func accObjectStorageAccessKeyMinimal(t *testing.T) {
	tt := newTestTools(t)
	name := strings.ToLower(tt.ResourceName())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy: testAccCheckKatapultObjectStorageAccessKeyDestroy(
			tt,
		),
		Steps: []resource.TestStep{
			{
				Config: objectStorageAccountDataBlock + undent.Stringf(`
					resource "katapult_object_storage_access_key" "main" {
					  name                      = "%s"
					  object_storage_account_id = data.katapult_object_storage_account.main.id
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultObjectStorageAccessKeyAttrs(
						tt,
						"katapult_object_storage_access_key.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_access_key.main",
						"name", name,
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_access_key.main",
						"object_storage_account_id",
						objectStorageAccTestRegion,
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_access_key.main",
						"all_buckets_read", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_access_key.main",
						"all_objects_read", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_access_key.main",
						"all_objects_write", "false",
					),
					resource.TestCheckResourceAttrSet(
						"katapult_object_storage_access_key.main",
						"access_key_id",
					),
					resource.TestCheckResourceAttrSet(
						"katapult_object_storage_access_key.main",
						"secret_access_key",
					),
					resource.TestCheckResourceAttrSet(
						"katapult_object_storage_access_key.main",
						"server_url",
					),
				),
			},
			{
				ResourceName:      "katapult_object_storage_access_key.main",
				ImportState:       true,
				ImportStateVerify: true,
				// secret_access_key is only returned by the API at
				// creation time and cannot be retrieved again after import.
				ImportStateVerifyIgnore: []string{"secret_access_key"},
			},
		},
	})
}

func accObjectStorageAccessKeyUpdateName(t *testing.T) {
	tt := newTestTools(t)
	name := strings.ToLower(tt.ResourceName())

	cfg := func(n string) string {
		return objectStorageAccountDataBlock + undent.Stringf(`
			resource "katapult_object_storage_access_key" "main" {
			  name                      = "%s"
			  object_storage_account_id = data.katapult_object_storage_account.main.id
			}`,
			n,
		)
	}

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy: testAccCheckKatapultObjectStorageAccessKeyDestroy(
			tt,
		),
		Steps: []resource.TestStep{
			{
				Config: cfg(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultObjectStorageAccessKeyAttrs(
						tt,
						"katapult_object_storage_access_key.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_access_key.main",
						"name", name,
					),
				),
			},
			{
				Config: cfg(name + "-updated"),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultObjectStorageAccessKeyAttrs(
						tt,
						"katapult_object_storage_access_key.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_access_key.main",
						"name", name+"-updated",
					),
					resource.TestCheckResourceAttrSet(
						"katapult_object_storage_access_key.main",
						"access_key_id",
					),
					resource.TestCheckResourceAttrSet(
						"katapult_object_storage_access_key.main",
						"secret_access_key",
					),
					resource.TestCheckResourceAttrSet(
						"katapult_object_storage_access_key.main",
						"server_url",
					),
				),
			},
		},
	})
}

func accObjectStorageAccessKeyBuckets(t *testing.T) {
	tt := newTestTools(t)
	baseName := strings.ToLower(tt.ResourceName())
	keyName := baseName
	bucketName := baseName + "-bkt"

	cfg := objectStorageAccountDataBlock + undent.Stringf(`
		resource "katapult_object_storage_access_key" "main" {
		  name                      = "%s"
		  object_storage_account_id = data.katapult_object_storage_account.main.id
		}

		resource "katapult_object_storage_bucket" "main" {
		  name                      = "%s"
		  object_storage_account_id = data.katapult_object_storage_account.main.id
		  read_key_ids              = [katapult_object_storage_access_key.main.id]
		  write_key_ids             = [katapult_object_storage_access_key.main.id]
		}`,
		keyName,
		bucketName,
	)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckKatapultObjectStorageAccessKeyDestroy(tt),
			testAccCheckKatapultObjectStorageBucketDestroy(tt),
		),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultObjectStorageAccessKeyAttrs(
						tt,
						"katapult_object_storage_access_key.main",
					),
					testAccCheckKatapultObjectStorageBucketAttrs(
						tt,
						"katapult_object_storage_bucket.main",
					),
				),
			},
			// Re-apply same config to trigger a Read refresh and verify
			// read_buckets/write_buckets are populated on the key.
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_object_storage_access_key.main",
						"read_buckets.*",
						"katapult_object_storage_bucket.main",
						"name",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_object_storage_access_key.main",
						"write_buckets.*",
						"katapult_object_storage_bucket.main",
						"name",
					),
				),
			},
		},
	})
}

func accObjectStorageAccessKeyUpdatePermissions(t *testing.T) {
	tt := newTestTools(t)
	name := strings.ToLower(tt.ResourceName())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy: testAccCheckKatapultObjectStorageAccessKeyDestroy(
			tt,
		),
		Steps: []resource.TestStep{
			{
				Config: objectStorageAccountDataBlock + undent.Stringf(`
					resource "katapult_object_storage_access_key" "main" {
					  name                      = "%s"
					  object_storage_account_id = data.katapult_object_storage_account.main.id
					  all_buckets_read          = true
					  all_objects_read          = true
					  all_objects_write         = true
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultObjectStorageAccessKeyAttrs(
						tt,
						"katapult_object_storage_access_key.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_access_key.main",
						"all_buckets_read", "true",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_access_key.main",
						"all_objects_read", "true",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_access_key.main",
						"all_objects_write", "true",
					),
				),
			},
			// Revoke all global permissions.
			{
				Config: objectStorageAccountDataBlock + undent.Stringf(`
					resource "katapult_object_storage_access_key" "main" {
					  name                      = "%s"
					  object_storage_account_id = data.katapult_object_storage_account.main.id
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultObjectStorageAccessKeyAttrs(
						tt,
						"katapult_object_storage_access_key.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_access_key.main",
						"all_buckets_read", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_access_key.main",
						"all_objects_read", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_access_key.main",
						"all_objects_write", "false",
					),
				),
			},
		},
	})
}

//
// Shared helpers
//

//nolint:unparam // res is designed to accept different resource names
func testAccCheckKatapultObjectStorageAccessKeyAttrs(
	tt *testTools,
	res string,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("resource not found: %s", res)
		}

		id := rs.Primary.Attributes["id"]

		resp, err := tt.Meta.Core.GetObjectStorageAccessKeyWithResponse(
			tt.Ctx,
			&core.GetObjectStorageAccessKeyParams{
				AccessKeyId: &id,
			},
		)
		if err != nil {
			return err
		}

		key := resp.JSON200.ObjectStorageAccessKey

		checks := []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(
				res, "id", *key.Id,
			),
			resource.TestCheckResourceAttr(
				res, "name", *key.Name,
			),
		}

		return resource.ComposeAggregateTestCheckFunc(checks...)(s)
	}
}

func testAccCheckKatapultObjectStorageAccessKeyDestroy(
	tt *testTools,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "katapult_object_storage_access_key" {
				continue
			}

			id := rs.Primary.Attributes["id"]

			resp, err := tt.Meta.Core.
				GetObjectStorageAccessKeyWithResponse(
					tt.Ctx,
					&core.GetObjectStorageAccessKeyParams{
						AccessKeyId: &id,
					},
				)
			if errors.Is(err, core.ErrNotFound) ||
				(resp != nil && resp.JSON404 != nil) {
				continue
			}

			if err != nil {
				return err
			}

			if resp == nil || resp.JSON200 == nil {
				return fmt.Errorf(
					"katapult_object_storage_access_key %s "+
						"returned unexpected response during destroy check",
					id,
				)
			}

			if resp.JSON404 == nil {
				return fmt.Errorf(
					"katapult_object_storage_access_key "+
						"%s still exists",
					id,
				)
			}
		}

		return nil
	}
}
