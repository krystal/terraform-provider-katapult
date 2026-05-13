package v6provider

import (
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/next/core"
	"github.com/stretchr/testify/require"
)

// TestAccKatapultObjectStorageAccount_lifecycle exercises the resource's
// full Create → Read → Import → Delete cycle against a clean org.
//
// This is a top-level test, NOT a subtest of TestAccKatapultObjectStorage,
// because it owns the account's full lifecycle (the umbrella test only
// references the account via the data source). Go runs top-level tests
// in the same package serially, so this won't race the umbrella test for
// ownership of the singleton account.
func TestAccKatapultObjectStorageAccount_lifecycle(t *testing.T) {
	tt := newTestTools(t)

	cfg := undent.Stringf(`
		resource "katapult_object_storage_account" "main" {
		  region = "%s"
		}`,
		objectStorageAccTestRegion,
	)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultObjectStorageAccountDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"katapult_object_storage_account.main",
						"id", objectStorageAccTestRegion,
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_account.main",
						"region", objectStorageAccTestRegion,
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_account.main",
						"adopt_existing", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_account.main",
						"provisioning_state", "provisioned",
					),
				),
			},
			// Refresh-only: any drift between Read and the previous plan
			// would show up as a non-empty plan here.
			{
				Config:             cfg,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
			// Import the same account and verify all attributes match.
			{
				ResourceName:      "katapult_object_storage_account.main",
				ImportState:       true,
				ImportStateId:     objectStorageAccTestRegion,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccKatapultObjectStorageAccount_adopt_existing verifies that
// `adopt_existing = true` silently takes ownership of an account created
// out-of-band rather than erroring on Create.
func TestAccKatapultObjectStorageAccount_adopt_existing(t *testing.T) {
	tt := newTestTools(t)

	// Pre-create the account directly via the SDK so the resource's Create
	// hits the "already exists" branch on first apply.
	preCreateObjectStorageAccountForTest(t, tt)

	cfg := undent.Stringf(`
		resource "katapult_object_storage_account" "main" {
		  region         = "%s"
		  adopt_existing = true
		}`,
		objectStorageAccTestRegion,
	)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultObjectStorageAccountDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"katapult_object_storage_account.main",
						"id", objectStorageAccTestRegion,
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_account.main",
						"adopt_existing", "true",
					),
					resource.TestCheckResourceAttr(
						"katapult_object_storage_account.main",
						"provisioning_state", "provisioned",
					),
				),
			},
			{
				Config:             cfg,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

// TestAccKatapultObjectStorageAccount_refuse_without_adopt verifies that
// applying without `adopt_existing` against an account that already exists
// fails with a diagnostic that points the user at `terraform import` and
// the `adopt_existing` flag.
func TestAccKatapultObjectStorageAccount_refuse_without_adopt(t *testing.T) {
	tt := newTestTools(t)

	preCreateObjectStorageAccountForTest(t, tt)
	// The resource's apply will fail, so framework won't try to destroy
	// from state — we need to clean up the out-of-band account ourselves.
	t.Cleanup(func() {
		deleteObjectStorageAccountForTest(t, tt)
	})

	cfg := undent.Stringf(`
		resource "katapult_object_storage_account" "main" {
		  region = "%s"
		}`,
		objectStorageAccTestRegion,
	)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						"An object storage account already exists",
					),
				),
			},
		},
	})
}

//
// Helpers
//

// preCreateObjectStorageAccountForTest creates the account directly via the
// SDK and waits for it to reach provisioned. Registers a t.Cleanup that
// will delete the account if it's still present at the end of the test.
func preCreateObjectStorageAccountForTest(t *testing.T, tt *testTools) {
	t.Helper()

	// If a previous test leaked an account, refuse to start rather than
	// trample over it.
	if _, err := getObjectStorageAccount(
		tt.Ctx, tt.Meta, objectStorageAccTestRegion,
	); err == nil {
		t.Fatalf(
			"object storage account already exists in region %s before "+
				"test start — clean it up before running this test",
			objectStorageAccTestRegion,
		)
	} else if !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("checking for pre-existing account: %s", err)
	}

	require.NoError(t, createObjectStorageAccount(
		tt.Ctx, tt.Meta, objectStorageAccTestRegion,
	))
	_, err := waitForObjectStorageAccountProvisioned(
		tt.Ctx, tt.Meta, objectStorageAccTestRegion,
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		// Belt-and-braces: if the test's resource.Test didn't destroy the
		// account (e.g. failed apply, or refuse-without-adopt test), nuke
		// it here so the next test starts clean.
		if _, err := getObjectStorageAccount(
			tt.Ctx, tt.Meta, objectStorageAccTestRegion,
		); errors.Is(err, core.ErrNotFound) {
			return
		}
		deleteObjectStorageAccountForTest(t, tt)
	})
}

func deleteObjectStorageAccountForTest(t *testing.T, tt *testTools) {
	t.Helper()

	region := objectStorageAccTestRegion
	delRes, err := tt.Meta.Core.
		DeleteOrganizationObjectStorageObjectStorageClusterWithResponse(
			tt.Ctx,
			core.DeleteOrganizationObjectStorageObjectStorageClusterJSONRequestBody{
				ObjectStorageCluster: core.ObjectStorageClusterLookup{
					Region: &region,
				},
				Organization: core.OrganizationLookup{
					SubDomain: &tt.Meta.confOrganization,
				},
			},
		)
	if err != nil && !errors.Is(err, core.ErrNotFound) {
		t.Errorf("cleanup delete failed: %s", err)
		return
	}

	if tt.Meta.SkipTrashObjectPurge {
		return
	}
	if delRes == nil || delRes.JSON200 == nil ||
		delRes.JSON200.TrashObject.Id == nil {
		return
	}
	trashID := *delRes.JSON200.TrashObject.Id
	if err := purgeTrashObject(
		tt.Ctx, tt.Meta, 5*time.Minute,
		core.TrashObject{Id: &trashID},
	); err != nil {
		t.Errorf("cleanup trash purge failed: %s", err)
	}
}

func testAccCheckKatapultObjectStorageAccountDestroy(
	tt *testTools,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		_, err := getObjectStorageAccount(
			tt.Ctx, tt.Meta, objectStorageAccTestRegion,
		)
		if errors.Is(err, core.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return errors.New(
			"katapult_object_storage_account still exists after destroy",
		)
	}
}
