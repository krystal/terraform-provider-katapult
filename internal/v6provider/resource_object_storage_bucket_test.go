package v6provider

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/next/core"
)

//
// Tests
//

func TestAccKatapultObjectStorageBucket_minimal(t *testing.T) {
	tt := newTestTools(t)
	name := strings.ToLower(tt.ResourceName())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy: testAccCheckKatapultObjectStorageBucketDestroy(
			tt,
		),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_object_storage_bucket" "main" {
					  name   = "%s"
					  region = "%s"
					}`,
					name,
					"uk-lon-1",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultObjectStorageBucketAttrs(
						tt, "katapult_object_storage_bucket.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_bucket.main",
						"name", name,
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_bucket.main",
						"region", "uk-lon-1",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_bucket.main",
						"serve_static_site", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_bucket.main",
						"all_keys_read", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_bucket.main",
						"all_keys_write", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_bucket.main",
						"public_list", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_bucket.main",
						"public_read", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_bucket.main",
						"read_key_ids.#", "0",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_bucket.main",
						"write_key_ids.#", "0",
					),
					resource.TestCheckResourceAttrSet(
						"katapult_object_storage_bucket.main",
						"public_url",
					),
				),
			},
		},
	})
}

func TestAccKatapultObjectStorageBucket_update_name(t *testing.T) {
	tt := newTestTools(t)
	name := strings.ToLower(tt.ResourceName())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy: testAccCheckKatapultObjectStorageBucketDestroy(
			tt,
		),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_object_storage_bucket" "main" {
					  name   = "%s"
					  region = "%s"
					}`,
					name,
					"uk-lon-1",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultObjectStorageBucketAttrs(
						tt, "katapult_object_storage_bucket.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_bucket.main",
						"name", name,
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_object_storage_bucket" "main" {
					  name   = "%s"
					  region = "%s"
					}`,
					name+"-updated",
					"uk-lon-1",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultObjectStorageBucketAttrs(
						tt, "katapult_object_storage_bucket.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_bucket.main",
						"name", name+"-updated",
					),
				),
			},
		},
	})
}

func TestAccKatapultObjectStorageBucket_update_label(t *testing.T) {
	tt := newTestTools(t)
	name := strings.ToLower(tt.ResourceName())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy: testAccCheckKatapultObjectStorageBucketDestroy(
			tt,
		),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_object_storage_bucket" "main" {
					  name   = "%s"
					  region = "%s"
					  label  = "My Bucket"
					}`,
					name,
					"uk-lon-1",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultObjectStorageBucketAttrs(
						tt, "katapult_object_storage_bucket.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_bucket.main",
						"label", "My Bucket",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_object_storage_bucket" "main" {
					  name   = "%s"
					  region = "%s"
					  label  = "Updated Bucket Label"
					}`,
					name,
					"uk-lon-1",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultObjectStorageBucketAttrs(
						tt, "katapult_object_storage_bucket.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_bucket.main",
						"label", "Updated Bucket Label",
					),
				),
			},
			// Remove the label entirely.
			{
				Config: undent.Stringf(`
					resource "katapult_object_storage_bucket" "main" {
					  name   = "%s"
					  region = "%s"
					}`,
					name,
					"uk-lon-1",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultObjectStorageBucketAttrs(
						tt, "katapult_object_storage_bucket.main",
					),
					resource.TestCheckNoResourceAttr(
						"katapult_object_storage_bucket.main",
						"label",
					),
				),
			},
		},
	})
}

func TestAccKatapultObjectStorageBucket_acl(t *testing.T) {
	tt := newTestTools(t)
	name := strings.ToLower(tt.ResourceName())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy: testAccCheckKatapultObjectStorageBucketDestroy(
			tt,
		),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_object_storage_bucket" "main" {
					  name           = "%s"
					  region         = "%s"
					  all_keys_read  = true
					  all_keys_write = true
					}`,
					name,
					"uk-lon-1",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultObjectStorageBucketAttrs(
						tt, "katapult_object_storage_bucket.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_bucket.main",
						"all_keys_read", "true",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_bucket.main",
						"all_keys_write", "true",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_bucket.main",
						"public_list", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_bucket.main",
						"public_read", "false",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_object_storage_bucket" "main" {
					  name        = "%s"
					  region      = "%s"
					  public_list = true
					  public_read = true
					}`,
					name,
					"uk-lon-1",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultObjectStorageBucketAttrs(
						tt, "katapult_object_storage_bucket.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_bucket.main",
						"all_keys_read", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_bucket.main",
						"all_keys_write", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_bucket.main",
						"public_list", "true",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_bucket.main",
						"public_read", "true",
					),
				),
			},
		},
	})
}

func TestAccKatapultObjectStorageBucket_static_site(t *testing.T) {
	tt := newTestTools(t)
	name := strings.ToLower(tt.ResourceName())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy: testAccCheckKatapultObjectStorageBucketDestroy(
			tt,
		),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_object_storage_bucket" "main" {
					  name              = "%s"
					  region            = "%s"
					  serve_static_site = true
					  static_site_index = "index.html"
					  static_site_error = "error.html"
					  public_list       = true
					  public_read       = true
					}`,
					name,
					"uk-lon-1",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultObjectStorageBucketAttrs(
						tt, "katapult_object_storage_bucket.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_bucket.main",
						"serve_static_site", "true",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_bucket.main",
						"static_site_index", "index.html",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_bucket.main",
						"static_site_error", "error.html",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_bucket.main",
						"public_list", "true",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_bucket.main",
						"public_read", "true",
					),
				),
			},
			// Update index and error pages while still serving.
			{
				Config: undent.Stringf(`
					resource "katapult_object_storage_bucket" "main" {
					  name              = "%s"
					  region            = "%s"
					  serve_static_site = true
					  static_site_index = "home.html"
					  static_site_error = "404.html"
					  public_list       = true
					  public_read       = true
					}`,
					name,
					"uk-lon-1",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultObjectStorageBucketAttrs(
						tt, "katapult_object_storage_bucket.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_bucket.main",
						"serve_static_site", "true",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_bucket.main",
						"static_site_index", "home.html",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_bucket.main",
						"static_site_error", "404.html",
					),
				),
			},
			// Disable static site serving.
			{
				Config: undent.Stringf(`
					resource "katapult_object_storage_bucket" "main" {
					  name              = "%s"
					  region            = "%s"
					  serve_static_site = false
					}`,
					name,
					"uk-lon-1",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultObjectStorageBucketAttrs(
						tt, "katapult_object_storage_bucket.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_bucket.main",
						"serve_static_site", "false",
					),
				),
			},
		},
	})
}

//
// Validation Tests
//

func TestAccKatapultObjectStorageBucket_validate_static_site_requires_index(t *testing.T) {
	tt := newTestTools(t)
	name := strings.ToLower(tt.ResourceName())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_object_storage_bucket" "main" {
					  name              = "%s"
					  region            = "%s"
					  serve_static_site = true
					  public_list       = true
					  public_read       = true
					}`,
					name,
					"uk-lon-1",
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						"Expected static_site_index to be present " +
							"when serve_static_site is true",
					),
				),
			},
		},
	})
}

func TestAccKatapultObjectStorageBucket_validate_static_site_requires_public_list(t *testing.T) {
	tt := newTestTools(t)
	name := strings.ToLower(tt.ResourceName())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_object_storage_bucket" "main" {
					  name              = "%s"
					  region            = "%s"
					  serve_static_site = true
					  static_site_index = "index.html"
					  public_read       = true
					}`,
					name,
					"uk-lon-1",
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						"Expected public_list to be true " +
							"when serve_static_site is true",
					),
				),
			},
		},
	})
}

func TestAccKatapultObjectStorageBucket_validate_static_site_requires_public_read(t *testing.T) {
	tt := newTestTools(t)
	name := strings.ToLower(tt.ResourceName())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_object_storage_bucket" "main" {
					  name              = "%s"
					  region            = "%s"
					  serve_static_site = true
					  static_site_index = "index.html"
					  public_list       = true
					}`,
					name,
					"uk-lon-1",
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						"Expected public_read to be true " +
							"when serve_static_site is true",
					),
				),
			},
		},
	})
}

func TestAccKatapultObjectStorageBucket_validate_static_site_index_forbidden(t *testing.T) {
	tt := newTestTools(t)
	name := strings.ToLower(tt.ResourceName())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_object_storage_bucket" "main" {
					  name              = "%s"
					  region            = "%s"
					  static_site_index = "index.html"
					}`,
					name,
					"uk-lon-1",
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						"Expected static_site_index to not be present " +
							"when serve_static_site is false",
					),
				),
			},
		},
	})
}

func TestAccKatapultObjectStorageBucket_validate_static_site_error_forbidden(t *testing.T) {
	tt := newTestTools(t)
	name := strings.ToLower(tt.ResourceName())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_object_storage_bucket" "main" {
					  name              = "%s"
					  region            = "%s"
					  static_site_error = "error.html"
					}`,
					name,
					"uk-lon-1",
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						"Expected static_site_error to not be present " +
							"when serve_static_site is false",
					),
				),
			},
		},
	})
}

//
// Helpers
//

//nolint:unparam // res is designed to accept different resource names
func testAccCheckKatapultObjectStorageBucketAttrs(
	tt *testTools,
	res string,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("resource not found: %s", res)
		}

		name := rs.Primary.Attributes["name"]
		region := rs.Primary.Attributes["region"]

		resp, err := tt.Meta.Core.
			GetObjectStorageObjectStorageClusterBucketWithResponse(
				tt.Ctx,
				&core.GetObjectStorageObjectStorageClusterBucketParams{
					BucketName:                 &name,
					ObjectStorageClusterRegion: &region,
				},
			)
		if err != nil {
			return err
		}

		b := resp.JSON200.ObjectStorageBucket

		checks := []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(res, "name", *b.Name),
			resource.TestCheckResourceAttr(res, "region", region),
		}

		if b.Label.IsSpecified() && !b.Label.IsNull() {
			checks = append(checks,
				resource.TestCheckResourceAttr(
					res, "label", b.Label.MustGet(),
				),
			)
		}

		if b.PublicUrl != nil {
			checks = append(checks,
				resource.TestCheckResourceAttr(
					res, "public_url", *b.PublicUrl,
				),
			)
		}

		if b.ServeStaticSite != nil {
			checks = append(checks,
				resource.TestCheckResourceAttr(
					res, "serve_static_site",
					strconv.FormatBool(*b.ServeStaticSite),
				),
			)
		}

		if b.StaticSiteIndex.IsSpecified() && !b.StaticSiteIndex.IsNull() {
			checks = append(checks,
				resource.TestCheckResourceAttr(
					res, "static_site_index",
					b.StaticSiteIndex.MustGet(),
				),
			)
		}

		if b.StaticSiteError.IsSpecified() && !b.StaticSiteError.IsNull() {
			checks = append(checks,
				resource.TestCheckResourceAttr(
					res, "static_site_error",
					b.StaticSiteError.MustGet(),
				),
			)
		}

		if acl := b.AccessControlList; acl != nil {
			if acl.AllKeysRead != nil {
				checks = append(checks,
					resource.TestCheckResourceAttr(
						res, "all_keys_read",
						strconv.FormatBool(*acl.AllKeysRead),
					),
				)
			}
			if acl.AllKeysWrite != nil {
				checks = append(checks,
					resource.TestCheckResourceAttr(
						res, "all_keys_write",
						strconv.FormatBool(*acl.AllKeysWrite),
					),
				)
			}
			if acl.PublicList != nil {
				checks = append(checks,
					resource.TestCheckResourceAttr(
						res, "public_list",
						strconv.FormatBool(*acl.PublicList),
					),
				)
			}
			if acl.PublicRead != nil {
				checks = append(checks,
					resource.TestCheckResourceAttr(
						res, "public_read",
						strconv.FormatBool(*acl.PublicRead),
					),
				)
			}
		}

		return resource.ComposeAggregateTestCheckFunc(checks...)(s)
	}
}

func testAccCheckKatapultObjectStorageBucketDestroy(
	tt *testTools,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "katapult_object_storage_bucket" {
				continue
			}

			name := rs.Primary.Attributes["name"]
			region := rs.Primary.Attributes["region"]

			resp, err := tt.Meta.Core.
				GetObjectStorageObjectStorageClusterBucketWithResponse(
					tt.Ctx,
					&core.GetObjectStorageObjectStorageClusterBucketParams{
						BucketName:                 &name,
						ObjectStorageClusterRegion: &region,
					},
				)
			if err == nil && resp.JSON404 == nil {
				return fmt.Errorf(
					"katapult_object_storage_bucket %s still exists",
					name,
				)
			}
		}

		return nil
	}
}
