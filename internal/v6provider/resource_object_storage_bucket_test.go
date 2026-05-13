package v6provider

import (
	"errors"
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

func accObjectStorageBucketMinimal(t *testing.T) {
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
				Config: objectStorageAccountDataBlock + undent.Stringf(`
					resource "katapult_object_storage_bucket" "main" {
					  name                      = "%s"
					  object_storage_account_id = data.katapult_object_storage_account.main.id
					}`,
					name,
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
						"object_storage_account_id",
						objectStorageAccTestRegion,
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
			{
				ResourceName: "katapult_object_storage_bucket.main",
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["katapult_object_storage_bucket.main"]
					if rs == nil {
						return "", fmt.Errorf("resource not found")
					}
					return rs.Primary.Attributes["name"] + "/" +
						rs.Primary.Attributes["object_storage_account_id"], nil
				},
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
			},
		},
	})
}

func accObjectStorageBucketUpdateName(t *testing.T) {
	tt := newTestTools(t)
	name := strings.ToLower(tt.ResourceName())

	cfg := func(n string) string {
		return objectStorageAccountDataBlock + undent.Stringf(`
			resource "katapult_object_storage_bucket" "main" {
			  name                      = "%s"
			  object_storage_account_id = data.katapult_object_storage_account.main.id
			}`,
			n,
		)
	}

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy: testAccCheckKatapultObjectStorageBucketDestroy(
			tt,
		),
		Steps: []resource.TestStep{
			{
				Config: cfg(name),
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
				Config: cfg(name + "-updated"),
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

func accObjectStorageBucketUpdateLabel(t *testing.T) {
	tt := newTestTools(t)
	name := strings.ToLower(tt.ResourceName())

	withLabel := func(label string) string {
		return objectStorageAccountDataBlock + undent.Stringf(`
			resource "katapult_object_storage_bucket" "main" {
			  name                      = "%s"
			  object_storage_account_id = data.katapult_object_storage_account.main.id
			  label                     = "%s"
			}`,
			name, label,
		)
	}

	noLabel := objectStorageAccountDataBlock + undent.Stringf(`
		resource "katapult_object_storage_bucket" "main" {
		  name                      = "%s"
		  object_storage_account_id = data.katapult_object_storage_account.main.id
		}`,
		name,
	)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy: testAccCheckKatapultObjectStorageBucketDestroy(
			tt,
		),
		Steps: []resource.TestStep{
			{
				Config: withLabel("My Bucket"),
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
				Config: withLabel("Updated Bucket Label"),
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
			{
				Config: noLabel,
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

func accObjectStorageBucketACL(t *testing.T) {
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
				Config: objectStorageAccountDataBlock + undent.Stringf(`
					resource "katapult_object_storage_bucket" "main" {
					  name                      = "%s"
					  object_storage_account_id = data.katapult_object_storage_account.main.id
					  all_keys_read             = true
					  all_keys_write            = true
					}`,
					name,
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
				Config: objectStorageAccountDataBlock + undent.Stringf(`
					resource "katapult_object_storage_bucket" "main" {
					  name                      = "%s"
					  object_storage_account_id = data.katapult_object_storage_account.main.id
					  public_list               = true
					  public_read               = true
					}`,
					name,
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

func accObjectStorageBucketStaticSite(t *testing.T) {
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
				Config: objectStorageAccountDataBlock + undent.Stringf(`
					resource "katapult_object_storage_bucket" "main" {
					  name                      = "%s"
					  object_storage_account_id = data.katapult_object_storage_account.main.id
					  serve_static_site         = true
					  static_site_index         = "index.html"
					  static_site_error         = "error.html"
					  public_list               = true
					  public_read               = true
					}`,
					name,
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
				Config: objectStorageAccountDataBlock + undent.Stringf(`
					resource "katapult_object_storage_bucket" "main" {
					  name                      = "%s"
					  object_storage_account_id = data.katapult_object_storage_account.main.id
					  serve_static_site         = true
					  static_site_index         = "home.html"
					  static_site_error         = "404.html"
					  public_list               = true
					  public_read               = true
					}`,
					name,
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
				Config: objectStorageAccountDataBlock + undent.Stringf(`
					resource "katapult_object_storage_bucket" "main" {
					  name                      = "%s"
					  object_storage_account_id = data.katapult_object_storage_account.main.id
					  serve_static_site         = false
					}`,
					name,
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
// Validation Tests (no HTTP)
//

func accObjectStorageBucketValidateStaticSiteRequiresIndex(t *testing.T) {
	tt := newTestTools(t).NoHTTP()
	name := strings.ToLower(tt.ResourceName())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_object_storage_bucket" "main" {
					  name                      = "%s"
					  object_storage_account_id = "%s"
					  serve_static_site         = true
					  public_list               = true
					  public_read               = true
					}`,
					name, objectStorageAccTestRegion,
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

func accObjectStorageBucketValidateStaticSiteRequiresPublicList(t *testing.T) {
	tt := newTestTools(t).NoHTTP()
	name := strings.ToLower(tt.ResourceName())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_object_storage_bucket" "main" {
					  name                      = "%s"
					  object_storage_account_id = "%s"
					  serve_static_site         = true
					  static_site_index         = "index.html"
					  public_read               = true
					}`,
					name, objectStorageAccTestRegion,
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

func accObjectStorageBucketValidateStaticSiteRequiresPublicRead(t *testing.T) {
	tt := newTestTools(t).NoHTTP()
	name := strings.ToLower(tt.ResourceName())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_object_storage_bucket" "main" {
					  name                      = "%s"
					  object_storage_account_id = "%s"
					  serve_static_site         = true
					  static_site_index         = "index.html"
					  public_list               = true
					}`,
					name, objectStorageAccTestRegion,
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

func accObjectStorageBucketValidateStaticSiteIndexForbidden(t *testing.T) {
	tt := newTestTools(t).NoHTTP()
	name := strings.ToLower(tt.ResourceName())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_object_storage_bucket" "main" {
					  name                      = "%s"
					  object_storage_account_id = "%s"
					  static_site_index         = "index.html"
					}`,
					name, objectStorageAccTestRegion,
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

func accObjectStorageBucketValidateStaticSiteErrorForbidden(t *testing.T) {
	tt := newTestTools(t).NoHTTP()
	name := strings.ToLower(tt.ResourceName())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_object_storage_bucket" "main" {
					  name                      = "%s"
					  object_storage_account_id = "%s"
					  static_site_error         = "error.html"
					}`,
					name, objectStorageAccTestRegion,
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
// Shared helpers
//

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
		region := rs.Primary.Attributes["object_storage_account_id"]

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
			resource.TestCheckResourceAttr(
				res, "object_storage_account_id", region,
			),
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
			region := rs.Primary.Attributes["object_storage_account_id"]

			resp, err := tt.Meta.Core.
				GetObjectStorageObjectStorageClusterBucketWithResponse(
					tt.Ctx,
					&core.GetObjectStorageObjectStorageClusterBucketParams{
						BucketName:                 &name,
						ObjectStorageClusterRegion: &region,
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
					"katapult_object_storage_bucket %s/%s "+
						"returned unexpected response during destroy check",
					name, region,
				)
			}

			return fmt.Errorf(
				"katapult_object_storage_bucket %s/%s still exists",
				name, region,
			)
		}

		return nil
	}
}
