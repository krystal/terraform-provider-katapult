package v6provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"
)

func TestObjectStorageAccessKeyCreateRetainsStateWhenCredentialsFail(
	t *testing.T,
) {
	var createCalls, credentialCalls atomic.Int32
	meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch req.URL.Path {
			case "/core/v1/organizations/organization/object_storage/" +
				"object_storage_cluster/access_keys":
				createCalls.Add(1)
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{
					"object_storage_access_key": {
						"id": "objkey_recoverable",
						"name": "recoverable-key",
						"region": "uk-lon-1"
					}
				}`))
			case "/core/v1/object_storage/access_keys/access_key/" +
				"generate_credentials":
				credentialCalls.Add(1)
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{
					"error": {
						"code": "credential_generation_failed",
						"description": "injected credential failure",
						"detail": {}
					}
				}`))
			default:
				http.Error(w, "unexpected request: "+req.URL.Path,
					http.StatusInternalServerError)
			}
		},
	))

	r := &ObjectStorageAccessKeyResource{M: meta}
	plan := ObjectStorageAccessKeyResourceModel{
		ID:              types.StringUnknown(),
		Name:            types.StringValue("recoverable-key"),
		Region:          types.StringValue("uk-lon-1"),
		AllBucketsRead:  types.BoolValue(true),
		AllObjectsRead:  types.BoolValue(false),
		AllObjectsWrite: types.BoolValue(true),
		ReadBuckets:     types.SetUnknown(types.StringType),
		WriteBuckets:    types.SetUnknown(types.StringType),
		AccessKeyID:     types.StringUnknown(),
		SecretAccessKey: types.StringUnknown(),
		ServerURL:       types.StringUnknown(),
	}
	req, resp := objectStorageCreateOperation(t, r.Schema, plan)

	r.Create(context.Background(), req, &resp)

	require.Equal(t, int32(1), createCalls.Load())
	require.Equal(t, int32(1), credentialCalls.Load())
	requireDiagnosticContains(t, resp.Diagnostics,
		"Object Storage Access Key Credentials Error")

	var state ObjectStorageAccessKeyResourceModel
	require.False(t, resp.State.Get(context.Background(), &state).HasError())
	require.Equal(t, "objkey_recoverable", state.ID.ValueString())
	require.Equal(t, "recoverable-key", state.Name.ValueString())
	require.Equal(t, "uk-lon-1", state.Region.ValueString())
	require.True(t, state.AllBucketsRead.ValueBool())
	require.False(t, state.AllObjectsRead.ValueBool())
	require.True(t, state.AllObjectsWrite.ValueBool())
	requireKnownNullString(t, state.AccessKeyID)
	requireKnownNullString(t, state.SecretAccessKey)
	requireKnownNullString(t, state.ServerURL)
	require.False(t, state.ReadBuckets.IsUnknown())
	require.False(t, state.ReadBuckets.IsNull())
	require.Empty(t, state.ReadBuckets.Elements())
	require.False(t, state.WriteBuckets.IsUnknown())
	require.False(t, state.WriteBuckets.IsNull())
	require.Empty(t, state.WriteBuckets.Elements())
}

func TestObjectStorageAccessKeyCreateRejectsIncompleteCredentials(
	t *testing.T,
) {
	testCases := map[string]struct {
		response string
		missing  string
	}{
		"missing access key ID": {
			response: `{
				"s3_secret_access_key": "secret",
				"server_url": "https://objects.example.test"
			}`,
			missing: "s3_access_key_id",
		},
		"null access key ID": {
			response: `{
				"s3_access_key_id": null,
				"s3_secret_access_key": "secret",
				"server_url": "https://objects.example.test"
			}`,
			missing: "s3_access_key_id",
		},
		"empty access key ID": {
			response: `{
				"s3_access_key_id": "",
				"s3_secret_access_key": "secret",
				"server_url": "https://objects.example.test"
			}`,
			missing: "s3_access_key_id",
		},
		"missing secret access key": {
			response: `{
				"s3_access_key_id": "access-key",
				"server_url": "https://objects.example.test"
			}`,
			missing: "s3_secret_access_key",
		},
		"null secret access key": {
			response: `{
				"s3_access_key_id": "access-key",
				"s3_secret_access_key": null,
				"server_url": "https://objects.example.test"
			}`,
			missing: "s3_secret_access_key",
		},
		"empty secret access key": {
			response: `{
				"s3_access_key_id": "access-key",
				"s3_secret_access_key": "",
				"server_url": "https://objects.example.test"
			}`,
			missing: "s3_secret_access_key",
		},
		"missing server URL": {
			response: `{
				"s3_access_key_id": "access-key",
				"s3_secret_access_key": "secret"
			}`,
			missing: "server_url",
		},
		"null server URL": {
			response: `{
				"s3_access_key_id": "access-key",
				"s3_secret_access_key": "secret",
				"server_url": null
			}`,
			missing: "server_url",
		},
		"empty server URL": {
			response: `{
				"s3_access_key_id": "access-key",
				"s3_secret_access_key": "secret",
				"server_url": ""
			}`,
			missing: "server_url",
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			var credentialCalls atomic.Int32
			meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
				func(w http.ResponseWriter, req *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					switch req.URL.Path {
					case "/core/v1/organizations/organization/object_storage/" +
						"object_storage_cluster/access_keys":
						w.WriteHeader(http.StatusCreated)
						_, _ = w.Write([]byte(`{
							"object_storage_access_key": {
								"id": "objkey_incomplete",
								"name": "incomplete-key",
								"region": "future-region"
							}
						}`))
					case "/core/v1/object_storage/access_keys/access_key/" +
						"generate_credentials":
						credentialCalls.Add(1)
						w.WriteHeader(http.StatusOK)
						_, _ = w.Write([]byte(
							`{"object_storage_access_key":` +
								testCase.response + `}`,
						))
					default:
						http.Error(w, "unexpected request: "+req.URL.Path,
							http.StatusInternalServerError)
					}
				},
			))

			r := &ObjectStorageAccessKeyResource{M: meta}
			plan := ObjectStorageAccessKeyResourceModel{
				ID:              types.StringUnknown(),
				Name:            types.StringValue("incomplete-key"),
				Region:          types.StringValue("future-region"),
				AllBucketsRead:  types.BoolValue(false),
				AllObjectsRead:  types.BoolValue(false),
				AllObjectsWrite: types.BoolValue(false),
				ReadBuckets:     types.SetUnknown(types.StringType),
				WriteBuckets:    types.SetUnknown(types.StringType),
				AccessKeyID:     types.StringUnknown(),
				SecretAccessKey: types.StringUnknown(),
				ServerURL:       types.StringUnknown(),
			}
			req, resp := objectStorageCreateOperation(t, r.Schema, plan)

			r.Create(context.Background(), req, &resp)

			require.Equal(t, int32(1), credentialCalls.Load())
			requireDiagnosticContains(t, resp.Diagnostics,
				"Object Storage Access Key Credentials Error")
			requireDiagnosticContains(t, resp.Diagnostics, testCase.missing)

			var state ObjectStorageAccessKeyResourceModel
			require.False(t,
				resp.State.Get(context.Background(), &state).HasError())
			require.Equal(t, "objkey_incomplete", state.ID.ValueString())
			require.Equal(t, "incomplete-key", state.Name.ValueString())
			require.Equal(t, "future-region", state.Region.ValueString())
			requireKnownNullString(t, state.AccessKeyID)
			requireKnownNullString(t, state.SecretAccessKey)
			requireKnownNullString(t, state.ServerURL)
		})
	}
}

func TestObjectStorageAccessKeyCreateAppliesOnlyCredentialFields(
	t *testing.T,
) {
	meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch req.URL.Path {
			case "/core/v1/organizations/organization/object_storage/" +
				"object_storage_cluster/access_keys":
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{
					"object_storage_access_key": {
						"id": "objkey_complete",
						"name": "complete-key",
						"region": "future-region"
					}
				}`))
			case "/core/v1/object_storage/access_keys/access_key/" +
				"generate_credentials":
				_, _ = w.Write([]byte(`{
					"object_storage_access_key": {
						"s3_access_key_id": "generated-access-key",
						"s3_secret_access_key": "generated-secret",
						"server_url": "https://objects.example.test"
					}
				}`))
			default:
				http.Error(w, "unexpected request", http.StatusInternalServerError)
			}
		},
	))

	r := &ObjectStorageAccessKeyResource{M: meta}
	plan := ObjectStorageAccessKeyResourceModel{
		ID:              types.StringUnknown(),
		Name:            types.StringValue("complete-key"),
		Region:          types.StringValue("future-region"),
		AllBucketsRead:  types.BoolValue(true),
		AllObjectsRead:  types.BoolValue(false),
		AllObjectsWrite: types.BoolValue(true),
		ReadBuckets:     types.SetUnknown(types.StringType),
		WriteBuckets:    types.SetUnknown(types.StringType),
		AccessKeyID:     types.StringUnknown(),
		SecretAccessKey: types.StringUnknown(),
		ServerURL:       types.StringUnknown(),
	}
	req, resp := objectStorageCreateOperation(t, r.Schema, plan)

	r.Create(context.Background(), req, &resp)

	require.False(t, resp.Diagnostics.HasError())
	var state ObjectStorageAccessKeyResourceModel
	require.False(t, resp.State.Get(context.Background(), &state).HasError())
	require.Equal(t, "objkey_complete", state.ID.ValueString())
	require.Equal(t, "complete-key", state.Name.ValueString())
	require.Equal(t, "future-region", state.Region.ValueString())
	require.False(t, state.AllBucketsRead.IsNull())
	require.False(t, state.AllBucketsRead.IsUnknown())
	require.True(t, state.AllBucketsRead.ValueBool())
	require.False(t, state.AllObjectsRead.IsNull())
	require.False(t, state.AllObjectsRead.IsUnknown())
	require.False(t, state.AllObjectsRead.ValueBool())
	require.False(t, state.AllObjectsWrite.IsNull())
	require.False(t, state.AllObjectsWrite.IsUnknown())
	require.True(t, state.AllObjectsWrite.ValueBool())
	require.False(t, state.ReadBuckets.IsNull())
	require.False(t, state.ReadBuckets.IsUnknown())
	require.Empty(t, state.ReadBuckets.Elements())
	require.False(t, state.WriteBuckets.IsNull())
	require.False(t, state.WriteBuckets.IsUnknown())
	require.Empty(t, state.WriteBuckets.Elements())
	require.Equal(t, "generated-access-key", state.AccessKeyID.ValueString())
	require.Equal(t, "generated-secret", state.SecretAccessKey.ValueString())
	require.Equal(t, "https://objects.example.test", state.ServerURL.ValueString())
}

func TestObjectStorageBucketCreateRetainsStateWhenReadBackFails(
	t *testing.T,
) {
	var createCalls, readCalls atomic.Int32
	meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch req.Method + " " + req.URL.Path {
			case http.MethodPost + " /core/v1/organizations/organization/" +
				"object_storage/object_storage_cluster/buckets":
				createCalls.Add(1)
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{
					"object_storage_bucket": {"name": "recoverable-bucket"}
				}`))
			case http.MethodGet + " /core/v1/object_storage/" +
				"object_storage_cluster/buckets/bucket":
				readCalls.Add(1)
				w.WriteHeader(http.StatusTeapot)
				_, _ = w.Write([]byte(`{"error":"injected read failure"}`))
			default:
				http.Error(w, "unexpected request: "+req.URL.Path,
					http.StatusInternalServerError)
			}
		},
	))

	r := &ObjectStorageBucketResource{M: meta}
	plan := ObjectStorageBucketResourceModel{
		Name:            types.StringValue("recoverable-bucket"),
		Region:          types.StringValue("uk-lon-1"),
		Label:           types.StringValue("Recoverable Bucket"),
		PublicURL:       types.StringUnknown(),
		ServeStaticSite: types.BoolValue(false),
		StaticSiteError: types.StringValue(""),
		StaticSiteIndex: types.StringValue(""),
		AllKeysRead:     types.BoolValue(true),
		AllKeysWrite:    types.BoolValue(false),
		PublicList:      types.BoolValue(false),
		PublicRead:      types.BoolValue(true),
		ReadKeyIDs:      buildStringSet([]string{"objkey_read"}),
		WriteKeyIDs:     buildStringSet([]string{"objkey_write"}),
	}
	req, resp := objectStorageCreateOperation(t, r.Schema, plan)

	r.Create(context.Background(), req, &resp)

	require.Equal(t, int32(1), createCalls.Load())
	require.Equal(t, int32(1), readCalls.Load())
	requireDiagnosticContains(t, resp.Diagnostics,
		"Object Storage Bucket Read Error")

	var state ObjectStorageBucketResourceModel
	require.False(t, resp.State.Get(context.Background(), &state).HasError())
	require.Equal(t, "recoverable-bucket", state.Name.ValueString())
	require.Equal(t, "uk-lon-1", state.Region.ValueString())
	require.Equal(t, "Recoverable Bucket", state.Label.ValueString())
	require.True(t, state.AllKeysRead.ValueBool())
	require.False(t, state.AllKeysWrite.ValueBool())
	require.False(t, state.PublicList.ValueBool())
	require.True(t, state.PublicRead.ValueBool())
	requireKnownNullString(t, state.PublicURL)
	require.False(t, state.ReadKeyIDs.IsUnknown())
	require.False(t, state.WriteKeyIDs.IsUnknown())
}

func TestObjectStorageBucketCreateRetainsStateWhenReadOmitsACL(
	t *testing.T,
) {
	meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch req.Method {
			case http.MethodPost:
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{
					"object_storage_bucket": {"name": "missing-acl"}
				}`))
			case http.MethodGet:
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{
					"object_storage_bucket": {
						"name": "missing-acl",
						"public_url": "https://objects.example.test/missing-acl",
						"serve_static_site": false
					}
				}`))
			default:
				http.Error(w, "unexpected request", http.StatusInternalServerError)
			}
		},
	))

	r := &ObjectStorageBucketResource{M: meta}
	plan := ObjectStorageBucketResourceModel{
		Name:            types.StringValue("missing-acl"),
		Region:          types.StringValue("future-region"),
		Label:           types.StringNull(),
		PublicURL:       types.StringUnknown(),
		ServeStaticSite: types.BoolValue(false),
		StaticSiteError: types.StringValue(""),
		StaticSiteIndex: types.StringValue(""),
		AllKeysRead:     types.BoolValue(false),
		AllKeysWrite:    types.BoolValue(false),
		PublicList:      types.BoolValue(false),
		PublicRead:      types.BoolValue(false),
		ReadKeyIDs:      buildStringSet(nil),
		WriteKeyIDs:     buildStringSet(nil),
	}
	req, resp := objectStorageCreateOperation(t, r.Schema, plan)

	r.Create(context.Background(), req, &resp)

	requireDiagnosticContains(t, resp.Diagnostics,
		"Object Storage Bucket Read Error")
	requireDiagnosticContains(t, resp.Diagnostics, "access_control_list")

	var state ObjectStorageBucketResourceModel
	require.False(t, resp.State.Get(context.Background(), &state).HasError())
	require.Equal(t, "missing-acl", state.Name.ValueString())
	require.Equal(t, "future-region", state.Region.ValueString())
	requireKnownNullString(t, state.PublicURL)
}

func TestObjectStorageAccountCreateRetainsStateWhenWaiterFails(
	t *testing.T,
) {
	var getCalls, createCalls atomic.Int32
	meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch req.Method {
			case http.MethodGet:
				call := getCalls.Add(1)
				if call == 1 {
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte(`{
						"error": {
							"code": "object_storage_account_not_found",
							"description": "not found",
							"detail": {}
						}
					}`))
					return
				}
				w.WriteHeader(http.StatusTeapot)
				_, _ = w.Write([]byte(`{"error":"injected waiter failure"}`))
			case http.MethodPost:
				createCalls.Add(1)
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{
					"object_storage_account": {
						"region": "uk-lon-1",
						"provisioning_state": "provisioning"
					}
				}`))
			default:
				http.Error(w, "unexpected method", http.StatusInternalServerError)
			}
		},
	))

	r := &ObjectStorageAccountResource{M: meta}
	plan := ObjectStorageAccountResourceModel{
		Region:            types.StringValue("uk-lon-1"),
		AdoptExisting:     types.BoolValue(false),
		ProvisioningState: types.StringUnknown(),
	}
	req, resp := objectStorageCreateOperation(t, r.Schema, plan)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	r.Create(ctx, req, &resp)

	require.Equal(t, int32(1), createCalls.Load())
	require.Equal(t, int32(2), getCalls.Load())
	requireDiagnosticContains(t, resp.Diagnostics,
		"Object Storage Account Provisioning Error")

	var state ObjectStorageAccountResourceModel
	require.False(t, resp.State.Get(context.Background(), &state).HasError())
	require.Equal(t, "uk-lon-1", state.Region.ValueString())
	require.False(t, state.AdoptExisting.ValueBool())
	requireKnownNullString(t, state.ProvisioningState)
}

func TestObjectStorageAccountDeleteResumesTrashPurgeFromPrivateState(
	t *testing.T,
) {
	var trashDeleteCalls, trashReadCalls, accountDeleteCalls atomic.Int32
	meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch req.Method + " " + req.URL.Path {
			case http.MethodDelete + " /core/v1/trash_objects/trash_object":
				call := trashDeleteCalls.Add(1)
				if call == 1 {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{
						"error": {
							"code": "injected_purge_failure",
							"description": "try again",
							"detail": {}
						}
					}`))
					return
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{
					"task": {"id": "task_purge", "status": "pending"}
				}`))
			case http.MethodGet + " /core/v1/trash_objects/trash_object":
				trashReadCalls.Add(1)
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{
					"error": {
						"code": "trash_object_not_found",
						"description": "not found",
						"detail": {}
					}
				}`))
			case http.MethodDelete + " /core/v1/organizations/organization/" +
				"object_storage/object_storage_cluster":
				accountDeleteCalls.Add(1)
				http.Error(w, "account delete must not be repeated",
					http.StatusInternalServerError)
			default:
				http.Error(w, "unexpected request: "+req.URL.Path,
					http.StatusInternalServerError)
			}
		},
	))
	meta.testMode = true
	r := &ObjectStorageAccountResource{M: meta}
	ctx := context.Background()

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	stateModel := ObjectStorageAccountResourceModel{
		Region:            types.StringValue("uk-lon-1"),
		AdoptExisting:     types.BoolValue(false),
		ProvisioningState: types.StringValue("provisioned"),
	}
	state := tfsdk.State{Schema: schemaResp.Schema}
	require.False(t, state.Set(ctx, &stateModel).HasError())

	req := resource.DeleteRequest{State: state}
	resp := resource.DeleteResponse{State: state}
	initializeResourcePrivateState(t, &req, &resp)
	encodedTrashID, err := encodeObjectStorageAccountTrashID("trsh_resume")
	require.NoError(t, err)
	require.False(t, req.Private.SetKey(
		ctx, objectStorageAccountTrashIDPrivateKey, encodedTrashID,
	).HasError())

	r.Delete(ctx, req, &resp)

	requireDiagnosticContains(t, resp.Diagnostics,
		"Failed to purge object storage account from trash")
	retained, diags := resp.Private.GetKey(
		ctx, objectStorageAccountTrashIDPrivateKey,
	)
	require.False(t, diags.HasError())
	require.Equal(t, encodedTrashID, retained)
	require.Equal(t, int32(1), trashDeleteCalls.Load())
	require.Equal(t, int32(0), accountDeleteCalls.Load())

	retryReq := resource.DeleteRequest{State: state, Private: resp.Private}
	retryResp := resource.DeleteResponse{State: state, Private: resp.Private}
	r.Delete(ctx, retryReq, &retryResp)

	require.False(t, retryResp.Diagnostics.HasError())
	cleared, diags := retryResp.Private.GetKey(
		ctx, objectStorageAccountTrashIDPrivateKey,
	)
	require.False(t, diags.HasError())
	require.Nil(t, cleared)
	require.Equal(t, int32(2), trashDeleteCalls.Load())
	require.Equal(t, int32(1), trashReadCalls.Load())
	require.Equal(t, int32(0), accountDeleteCalls.Load())
}

func TestObjectStorageAccountReadRetainsStateForPendingTrashPurge(
	t *testing.T,
) {
	var accountReadCalls atomic.Int32
	meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if req.Method != http.MethodGet || req.URL.Path !=
				"/core/v1/organizations/organization/object_storage/"+
					"object_storage_cluster" {
				http.Error(w, "unexpected request", http.StatusInternalServerError)
				return
			}
			accountReadCalls.Add(1)
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{
				"error": {
					"code": "object_storage_account_not_found",
					"description": "not found",
					"detail": {}
				}
			}`))
		},
	))
	r := &ObjectStorageAccountResource{M: meta}
	ctx := context.Background()

	state := objectStorageAccountTestState(t, r)
	req := resource.ReadRequest{State: state}
	resp := resource.ReadResponse{State: state}
	initializeResourcePrivateState(t, &req, &resp)
	encodedTrashID, err := encodeObjectStorageAccountTrashID("trsh_pending")
	require.NoError(t, err)
	require.False(t, req.Private.SetKey(
		ctx, objectStorageAccountTrashIDPrivateKey, encodedTrashID,
	).HasError())

	r.Read(ctx, req, &resp)

	require.False(t, resp.Diagnostics.HasError())
	require.Equal(t, int32(1), accountReadCalls.Load())
	var retained ObjectStorageAccountResourceModel
	require.False(t, resp.State.Get(ctx, &retained).HasError())
	require.Equal(t, "uk-lon-1", retained.Region.ValueString())
	require.Equal(t, "provisioned", retained.ProvisioningState.ValueString())
	privateTrashID, diags := resp.Private.GetKey(
		ctx, objectStorageAccountTrashIDPrivateKey,
	)
	require.False(t, diags.HasError())
	require.Equal(t, encodedTrashID, privateTrashID)
}

func TestObjectStorageAccountReadClearsStaleTrashBeforeDelete(
	t *testing.T,
) {
	var accountReadCalls, keyListCalls, accountDeleteCalls atomic.Int32
	meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch req.Method + " " + req.URL.Path {
			case http.MethodGet + " /core/v1/organizations/organization/" +
				"object_storage/object_storage_cluster":
				accountReadCalls.Add(1)
				_, _ = w.Write([]byte(`{
					"object_storage_account": {
						"region": "uk-lon-1",
						"bucket_count": 0,
						"provisioning_state": "provisioned"
					}
				}`))
			case http.MethodGet + " /core/v1/organizations/organization/" +
				"object_storage/access_keys":
				keyListCalls.Add(1)
				_, _ = w.Write([]byte(`{
					"pagination": {
						"current_page": 1,
						"total_pages": 1,
						"per_page": 100,
						"large_set": false
					},
					"object_storage_access_keys": []
				}`))
			case http.MethodDelete + " /core/v1/organizations/organization/" +
				"object_storage/object_storage_cluster":
				accountDeleteCalls.Add(1)
				_, _ = w.Write([]byte(`{
					"object_storage_account": {"region": "uk-lon-1"},
					"trash_object": {"id": "trsh_live_account"}
				}`))
			default:
				http.Error(w, "unexpected request: "+req.URL.Path,
					http.StatusInternalServerError)
			}
		},
	))
	meta.SkipTrashObjectPurge = true
	r := &ObjectStorageAccountResource{M: meta}
	ctx := context.Background()
	state := objectStorageAccountTestState(t, r)
	readReq := resource.ReadRequest{State: state}
	readResp := resource.ReadResponse{State: state}
	initializeResourcePrivateState(t, &readReq, &readResp)
	encodedTrashID, err := encodeObjectStorageAccountTrashID("trsh_stale")
	require.NoError(t, err)
	require.False(t, readReq.Private.SetKey(
		ctx, objectStorageAccountTrashIDPrivateKey, encodedTrashID,
	).HasError())

	r.Read(ctx, readReq, &readResp)

	require.False(t, readResp.Diagnostics.HasError())
	cleared, diags := readResp.Private.GetKey(
		ctx, objectStorageAccountTrashIDPrivateKey,
	)
	require.False(t, diags.HasError())
	require.Nil(t, cleared)

	deleteReq := resource.DeleteRequest{
		State: readResp.State, Private: readResp.Private,
	}
	deleteResp := resource.DeleteResponse{
		State: readResp.State, Private: readResp.Private,
	}
	r.Delete(ctx, deleteReq, &deleteResp)

	require.False(t, deleteResp.Diagnostics.HasError())
	require.Equal(t, int32(2), accountReadCalls.Load())
	require.Equal(t, int32(1), keyListCalls.Load())
	require.Equal(t, int32(1), accountDeleteCalls.Load())
}

func TestObjectStorageAccountDeleteClearsPrivateStateWhenTrashAlreadyGone(
	t *testing.T,
) {
	var trashDeleteCalls, unexpectedCalls atomic.Int32
	meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if req.Method == http.MethodDelete && req.URL.Path ==
				"/core/v1/trash_objects/trash_object" {
				trashDeleteCalls.Add(1)
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{
					"error": {
						"code": "trash_object_not_found",
						"description": "already gone",
						"detail": {}
					}
				}`))
				return
			}
			unexpectedCalls.Add(1)
			http.Error(w, "unexpected request", http.StatusInternalServerError)
		},
	))
	r := &ObjectStorageAccountResource{M: meta}
	ctx := context.Background()
	state := objectStorageAccountTestState(t, r)
	req := resource.DeleteRequest{State: state}
	resp := resource.DeleteResponse{State: state}
	initializeResourcePrivateState(t, &req, &resp)
	encodedTrashID, err := encodeObjectStorageAccountTrashID("trsh_gone")
	require.NoError(t, err)
	require.False(t, req.Private.SetKey(
		ctx, objectStorageAccountTrashIDPrivateKey, encodedTrashID,
	).HasError())

	r.Delete(ctx, req, &resp)

	require.False(t, resp.Diagnostics.HasError())
	require.Equal(t, int32(1), trashDeleteCalls.Load())
	require.Equal(t, int32(0), unexpectedCalls.Load())
	cleared, diags := resp.Private.GetKey(
		ctx, objectStorageAccountTrashIDPrivateKey,
	)
	require.False(t, diags.HasError())
	require.Nil(t, cleared)
}

type objectStorageSchemaFunc func(
	context.Context,
	resource.SchemaRequest,
	*resource.SchemaResponse,
)

func objectStorageCreateOperation(
	t *testing.T,
	schemaFunc objectStorageSchemaFunc,
	model any,
) (resource.CreateRequest, resource.CreateResponse) {
	t.Helper()
	ctx := context.Background()
	var schemaResp resource.SchemaResponse
	schemaFunc(ctx, resource.SchemaRequest{}, &schemaResp)

	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	require.False(t, plan.Set(ctx, model).HasError())

	return resource.CreateRequest{Plan: plan}, resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: plan.Raw},
	}
}

func newObjectStorageFailureTestMeta(
	t *testing.T,
	handler http.Handler,
) *Meta {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	t.Setenv("KATAPULT_TF_DEBUG_API_URL", server.URL)

	meta, err := NewMeta(
		"test-api-key", "test-data-center", "test-organization", nil,
		"error", "test", server.Client(), "test", "test",
	)
	require.NoError(t, err)
	meta.retryClient.RetryMax = 0

	return meta
}

func initializeResourcePrivateState(
	t *testing.T,
	req any,
	resp any,
) {
	t.Helper()

	// The framework exposes Private on resource requests and responses using an
	// internal type with no public constructor. Reflection is required here to
	// initialize it from provider tests, so validate the field shape explicitly
	// to make framework compatibility failures actionable.
	reqPrivate := reflect.ValueOf(req).Elem().FieldByName("Private")
	require.True(t, reqPrivate.IsValid(), "request has no Private field")
	require.True(t, reqPrivate.CanSet(), "request Private field cannot be set")
	require.Equal(t, reflect.Pointer, reqPrivate.Kind(),
		"request Private field is not a pointer")
	privateData := reflect.New(reqPrivate.Type().Elem())
	reqPrivate.Set(privateData)

	respPrivate := reflect.ValueOf(resp).Elem().FieldByName("Private")
	require.True(t, respPrivate.IsValid(), "response has no Private field")
	require.True(t, respPrivate.CanSet(), "response Private field cannot be set")
	require.Equal(t, reqPrivate.Type(), respPrivate.Type(),
		"request and response Private fields have different types")
	respPrivate.Set(privateData)
}

func objectStorageAccountTestState(
	t *testing.T,
	r *ObjectStorageAccountResource,
) tfsdk.State {
	t.Helper()
	ctx := context.Background()
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema}
	require.False(t, state.Set(ctx, &ObjectStorageAccountResourceModel{
		Region:            types.StringValue("uk-lon-1"),
		AdoptExisting:     types.BoolValue(false),
		ProvisioningState: types.StringValue("provisioned"),
	}).HasError())

	return state
}

func requireDiagnosticContains(
	t *testing.T,
	diagnostics diag.Diagnostics,
	needle string,
) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic.Summary(), needle) ||
			strings.Contains(diagnostic.Detail(), needle) {
			return
		}
	}
	require.Failf(t, "expected diagnostic not found", "%q in %#v",
		needle, diagnostics)
}

func requireKnownNullString(t *testing.T, value types.String) {
	t.Helper()
	require.True(t, value.IsNull())
	require.False(t, value.IsUnknown())
}
