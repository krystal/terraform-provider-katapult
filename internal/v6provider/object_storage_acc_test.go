package v6provider

import (
	"errors"
	"testing"
	"time"

	"github.com/krystal/go-katapult/next/core"
	"github.com/stretchr/testify/require"
)

const objectStorageAccTestRegion = "uk-lon-1"

// TestAccKatapultObjectStorage is the umbrella acceptance test for all
// object storage scenarios. It provisions a single shared object storage
// account for the organization (via SDK calls, not the Terraform resource
// itself), runs every scenario as a subtest against that shared account,
// and tears the account down once all subtests complete.
//
// Each scenario lives in its own test file as an unexported
// `func(t *testing.T)`; this file just orchestrates them. Subtests
// reference the shared account through the
// `data.katapult_object_storage_account` data source, mirroring the
// pattern production users follow when a different Terraform configuration
// manages the account.
//
// Run a single scenario via, e.g.:
//
//	go test -run TestAccKatapultObjectStorage_scenarios/AccessKey_minimal
//
// Each scenario records its own VCR cassette under
// `testdata/ObjectStorage_scenarios/<name>.cassette.yaml`. The parent's
// setup/teardown HTTP traffic is recorded to
// `testdata/ObjectStorage_scenarios.cassette.yaml`.
func TestAccKatapultObjectStorage_scenarios(t *testing.T) {
	parentTT := newTestTools(t)
	setupSharedObjectStorageAccount(t, parentTT)
	t.Cleanup(func() { teardownSharedObjectStorageAccount(t, parentTT) })

	t.Run("AccessKey_minimal", accObjectStorageAccessKeyMinimal)
	t.Run("AccessKey_buckets", accObjectStorageAccessKeyBuckets)
	t.Run("AccessKey_update_name", accObjectStorageAccessKeyUpdateName)
	t.Run("AccessKey_update_permissions",
		accObjectStorageAccessKeyUpdatePermissions)

	t.Run("Bucket_minimal", accObjectStorageBucketMinimal)
	t.Run("Bucket_acl", accObjectStorageBucketACL)
	t.Run("Bucket_static_site", accObjectStorageBucketStaticSite)
	t.Run("Bucket_update_label", accObjectStorageBucketUpdateLabel)
	t.Run("Bucket_update_name", accObjectStorageBucketUpdateName)
	t.Run("Bucket_validate_static_site_requires_index",
		accObjectStorageBucketValidateStaticSiteRequiresIndex)
	t.Run("Bucket_validate_static_site_requires_public_list",
		accObjectStorageBucketValidateStaticSiteRequiresPublicList)
	t.Run("Bucket_validate_static_site_requires_public_read",
		accObjectStorageBucketValidateStaticSiteRequiresPublicRead)
	t.Run("Bucket_validate_static_site_index_forbidden",
		accObjectStorageBucketValidateStaticSiteIndexForbidden)
	t.Run("Bucket_validate_static_site_error_forbidden",
		accObjectStorageBucketValidateStaticSiteErrorForbidden)

	t.Run("DataSource_Bucket_minimal",
		accDataSourceObjectStorageBucketMinimal)
	t.Run("DataSource_Bucket_not_found",
		accDataSourceObjectStorageBucketNotFound)
}

// setupSharedObjectStorageAccount creates the object storage account for the
// shared test region if it does not already exist, then waits for it to
// reach `provisioned`. This uses the same SDK helpers the
// katapult_object_storage_account resource uses internally.
func setupSharedObjectStorageAccount(t *testing.T, tt *testTools) {
	t.Helper()

	_, err := getObjectStorageAccount(
		tt.Ctx, tt.Meta, objectStorageAccTestRegion,
	)
	switch {
	case err == nil:
		// Account already exists — leave it in place; teardown will deal.
	case errors.Is(err, core.ErrNotFound):
		require.NoError(t, createObjectStorageAccount(
			tt.Ctx, tt.Meta, objectStorageAccTestRegion,
		))
	default:
		require.NoError(t, err, "fetching object storage account")
	}

	_, err = waitForObjectStorageAccountProvisioned(
		tt.Ctx, tt.Meta, objectStorageAccTestRegion,
	)
	require.NoError(t, err, "waiting for object storage account to provision")
}

// teardownSharedObjectStorageAccount deletes the shared account once
// all subtests have completed. Calls the preflight helper first so the
// failure mode (leftover buckets/keys from a broken subtest) is loud and
// actionable rather than a silent partial-cleanup.
func teardownSharedObjectStorageAccount(t *testing.T, tt *testTools) {
	t.Helper()

	if err := preflightObjectStorageAccountDelete(
		tt.Ctx, tt.Meta, objectStorageAccTestRegion,
	); err != nil {
		t.Errorf("object storage account preflight failed: %s", err)
		return
	}

	delRes, err := tt.Meta.Core.
		DeleteOrganizationObjectStorageObjectStorageClusterWithResponse(
			tt.Ctx,
			core.DeleteOrganizationObjectStorageObjectStorageClusterJSONRequestBody{
				ObjectStorageCluster: core.ObjectStorageClusterLookup{
					Region: stringPtr(objectStorageAccTestRegion),
				},
				Organization: core.OrganizationLookup{
					SubDomain: &tt.Meta.confOrganization,
				},
			},
		)
	if err != nil && !errors.Is(err, core.ErrNotFound) {
		t.Errorf("object storage account teardown failed: %s", err)
		return
	}

	// Mirror the resource's Delete behavior: purge the trash object unless
	// SkipTrashObjectPurge is set on the provider. Keeps the test org
	// clean between record runs.
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
		t.Errorf("object storage account trash purge failed: %s", err)
	}
}

func stringPtr(s string) *string { return &s }
