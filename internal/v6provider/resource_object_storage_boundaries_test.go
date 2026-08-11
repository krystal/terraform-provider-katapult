package v6provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
	"github.com/stretchr/testify/require"
)

func TestObjectStorageAccessKeyReadPreservesOmittedFieldsAndClearsEmptySet(
	t *testing.T,
) {
	meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			require.Equal(t, http.MethodGet, req.Method)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"object_storage_access_key": {
					"read_buckets": []
				}
			}`))
		},
	))
	r := &ObjectStorageAccessKeyResource{M: meta}
	ctx := context.Background()
	stateModel := objectStorageAccessKeyBoundaryModel()
	req, resp := objectStorageReadOperation(t, r.Schema, stateModel)

	r.Read(ctx, req, &resp)

	require.False(t, resp.Diagnostics.HasError())
	var result ObjectStorageAccessKeyResourceModel
	require.False(t, resp.State.Get(ctx, &result).HasError())
	require.Equal(t, stateModel.ID, result.ID)
	require.Equal(t, stateModel.Name, result.Name)
	require.Equal(t, stateModel.Region, result.Region)
	require.Equal(t, stateModel.AllBucketsRead, result.AllBucketsRead)
	require.Equal(t, stateModel.AllObjectsRead, result.AllObjectsRead)
	require.Equal(t, stateModel.AllObjectsWrite, result.AllObjectsWrite)
	require.Empty(t, result.ReadBuckets.Elements())
	require.True(t, result.WriteBuckets.Equal(stateModel.WriteBuckets))
	require.Equal(t, stateModel.AccessKeyID, result.AccessKeyID)
	require.Equal(t, stateModel.SecretAccessKey, result.SecretAccessKey)
	require.Equal(t, stateModel.ServerURL, result.ServerURL)
}

func TestObjectStorageAccessKeyUpdatePreservesOmittedFieldsAndClearsEmptySet(
	t *testing.T,
) {
	meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			require.Equal(t, http.MethodPatch, req.Method)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"object_storage_access_key": {
					"write_buckets": []
				}
			}`))
		},
	))
	r := &ObjectStorageAccessKeyResource{M: meta}
	ctx := context.Background()
	stateModel := objectStorageAccessKeyBoundaryModel()
	planModel := stateModel
	planModel.ID = types.StringUnknown()
	planModel.Name = types.StringValue("updated-key")
	planModel.AllObjectsRead = types.BoolValue(true)
	planModel.ReadBuckets = types.SetUnknown(types.StringType)
	planModel.WriteBuckets = types.SetUnknown(types.StringType)
	planModel.AccessKeyID = types.StringUnknown()
	planModel.SecretAccessKey = types.StringUnknown()
	planModel.ServerURL = types.StringUnknown()
	req, resp := objectStorageUpdateOperation(
		t, r.Schema, stateModel, planModel,
	)

	r.Update(ctx, req, &resp)

	require.False(t, resp.Diagnostics.HasError())
	var result ObjectStorageAccessKeyResourceModel
	require.False(t, resp.State.Get(ctx, &result).HasError())
	require.Equal(t, stateModel.ID, result.ID)
	require.Equal(t, planModel.Name, result.Name)
	require.Equal(t, stateModel.Region, result.Region)
	require.Equal(t, stateModel.AllBucketsRead, result.AllBucketsRead)
	require.Equal(t, planModel.AllObjectsRead, result.AllObjectsRead)
	require.Equal(t, stateModel.AllObjectsWrite, result.AllObjectsWrite)
	require.True(t, result.ReadBuckets.Equal(stateModel.ReadBuckets))
	require.Empty(t, result.WriteBuckets.Elements())
	require.Equal(t, stateModel.AccessKeyID, result.AccessKeyID)
	require.Equal(t, stateModel.SecretAccessKey, result.SecretAccessKey)
	require.Equal(t, stateModel.ServerURL, result.ServerURL)
}

func TestObjectStorageBucketReadPreservesOmittedFieldsAndClearsEmptySet(
	t *testing.T,
) {
	meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			require.Equal(t, http.MethodGet, req.Method)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"object_storage_bucket": {
					"access_control_list": {
						"read_key_ids": []
					}
				}
			}`))
		},
	))
	r := &ObjectStorageBucketResource{M: meta}
	ctx := context.Background()
	stateModel := objectStorageBucketPartialPayloadModel()
	req, resp := objectStorageReadOperation(t, r.Schema, stateModel)

	r.Read(ctx, req, &resp)

	require.False(t, resp.Diagnostics.HasError())
	var result ObjectStorageBucketResourceModel
	require.False(t, resp.State.Get(ctx, &result).HasError())
	require.Equal(t, stateModel.Name, result.Name)
	require.Equal(t, stateModel.Region, result.Region)
	require.Equal(t, stateModel.Label, result.Label)
	require.Equal(t, stateModel.PublicURL, result.PublicURL)
	require.Equal(t, stateModel.ServeStaticSite, result.ServeStaticSite)
	require.Equal(t, stateModel.StaticSiteError, result.StaticSiteError)
	require.Equal(t, stateModel.StaticSiteIndex, result.StaticSiteIndex)
	require.Equal(t, stateModel.AllKeysRead, result.AllKeysRead)
	require.Equal(t, stateModel.AllKeysWrite, result.AllKeysWrite)
	require.Equal(t, stateModel.PublicList, result.PublicList)
	require.Equal(t, stateModel.PublicRead, result.PublicRead)
	require.Empty(t, result.ReadKeyIDs.Elements())
	require.True(t, result.WriteKeyIDs.Equal(stateModel.WriteKeyIDs))
}

func TestObjectStorageBucketUpdatePreservesOmittedFieldsAndClearsEmptySet(
	t *testing.T,
) {
	requestCount := 0
	meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			requestCount++
			w.Header().Set("Content-Type", "application/json")
			switch req.Method {
			case http.MethodPatch:
				_, _ = w.Write([]byte(`{"object_storage_bucket": {}}`))
			case http.MethodGet:
				_, _ = w.Write([]byte(`{
					"object_storage_bucket": {
						"access_control_list": {
							"write_key_ids": []
						}
					}
				}`))
			default:
				http.Error(w, "unexpected request", http.StatusInternalServerError)
			}
		},
	))
	r := &ObjectStorageBucketResource{M: meta}
	ctx := context.Background()
	stateModel := objectStorageBucketPartialPayloadModel()
	planModel := stateModel
	planModel.Label = types.StringValue("updated label")
	planModel.PublicURL = types.StringUnknown()
	planModel.PublicRead = types.BoolValue(false)
	req, resp := objectStorageUpdateOperation(
		t, r.Schema, stateModel, planModel,
	)

	r.Update(ctx, req, &resp)

	require.False(t, resp.Diagnostics.HasError())
	require.Equal(t, 2, requestCount)
	var result ObjectStorageBucketResourceModel
	require.False(t, resp.State.Get(ctx, &result).HasError())
	require.Equal(t, stateModel.Name, result.Name)
	require.Equal(t, stateModel.Region, result.Region)
	require.Equal(t, planModel.Label, result.Label)
	require.Equal(t, stateModel.PublicURL, result.PublicURL)
	require.Equal(t, stateModel.ServeStaticSite, result.ServeStaticSite)
	require.Equal(t, stateModel.StaticSiteError, result.StaticSiteError)
	require.Equal(t, stateModel.StaticSiteIndex, result.StaticSiteIndex)
	require.Equal(t, stateModel.AllKeysRead, result.AllKeysRead)
	require.Equal(t, stateModel.AllKeysWrite, result.AllKeysWrite)
	require.Equal(t, stateModel.PublicList, result.PublicList)
	require.Equal(t, planModel.PublicRead, result.PublicRead)
	require.True(t, result.ReadKeyIDs.Equal(stateModel.ReadKeyIDs))
	require.Empty(t, result.WriteKeyIDs.Elements())
}

func TestObjectStorageBucketUpdateSendsEmptyKeySets(t *testing.T) {
	var (
		serverMu       sync.Mutex
		patchArgs      core.PatchObjectStorageObjectStorageClusterBucketJSONRequestBody
		serverReadIDs  = []string{"objkey_read"}
		serverWriteIDs = []string{"objkey_write"}
	)
	meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch req.Method {
			case http.MethodPatch:
				var received core.
					PatchObjectStorageObjectStorageClusterBucketJSONRequestBody
				if err := json.NewDecoder(req.Body).Decode(&received); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				serverMu.Lock()
				patchArgs = received
				acl := received.Properties.AccessControlList
				if acl != nil && acl.ReadKeyIds != nil {
					serverReadIDs = append([]string(nil), (*acl.ReadKeyIds)...)
				}
				if acl != nil && acl.WriteKeyIds != nil {
					serverWriteIDs = append([]string(nil), (*acl.WriteKeyIds)...)
				}
				serverMu.Unlock()
				_, _ = w.Write([]byte(`{
					"object_storage_bucket": {"name": "key-removal"}
				}`))
			case http.MethodGet:
				serverMu.Lock()
				readIDs := append([]string(nil), serverReadIDs...)
				writeIDs := append([]string(nil), serverWriteIDs...)
				serverMu.Unlock()
				_ = json.NewEncoder(w).Encode(map[string]any{
					"object_storage_bucket": map[string]any{
						"name":              "key-removal",
						"public_url":        "https://objects.example.test/key-removal",
						"serve_static_site": false,
						"access_control_list": map[string]any{
							"all_keys_read":  false,
							"all_keys_write": false,
							"public_list":    false,
							"public_read":    false,
							"read_key_ids":   readIDs,
							"write_key_ids":  writeIDs,
						},
					},
				})
			default:
				http.Error(w, "unexpected request", http.StatusInternalServerError)
			}
		},
	))

	r := &ObjectStorageBucketResource{M: meta}
	stateModel := objectStorageBucketBoundaryModel(
		buildStringSet([]string{"objkey_read"}),
		buildStringSet([]string{"objkey_write"}),
	)
	planModel := objectStorageBucketBoundaryModel(
		buildStringSet(nil), buildStringSet(nil),
	)
	req, resp := objectStorageUpdateOperation(
		t, r.Schema, stateModel, planModel,
	)

	r.Update(context.Background(), req, &resp)

	require.False(t, resp.Diagnostics.HasError())
	serverMu.Lock()
	receivedACL := patchArgs.Properties.AccessControlList
	serverMu.Unlock()
	require.NotNil(t, receivedACL)
	require.NotNil(t, receivedACL.ReadKeyIds)
	require.Empty(t, *receivedACL.ReadKeyIds)
	require.NotNil(t, receivedACL.WriteKeyIds)
	require.Empty(t, *receivedACL.WriteKeyIds)

	var result ObjectStorageBucketResourceModel
	require.False(t, resp.State.Get(context.Background(), &result).HasError())
	require.Empty(t, result.ReadKeyIDs.Elements())
	require.Empty(t, result.WriteKeyIDs.Elements())
}

func TestObjectStorageBucketCreatePropagatesInvalidRegionError(
	t *testing.T,
) {
	var requestBody []byte
	meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			requestBody, _ = io.ReadAll(req.Body)
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{
				"error": {
					"code": "validation_error",
					"description": "A validation error occurred",
					"detail": {"errors": ["Region is unavailable"]}
				}
			}`))
		},
	))

	r := &ObjectStorageBucketResource{M: meta}
	plan := objectStorageBucketBoundaryModel(
		buildStringSet(nil), buildStringSet(nil),
	)
	plan.Region = types.StringValue("future-region")
	req, resp := objectStorageCreateOperation(t, r.Schema, plan)

	r.Create(context.Background(), req, &resp)

	requireDiagnosticContains(t, resp.Diagnostics,
		"error creating object storage bucket")
	requireDiagnosticContains(t, resp.Diagnostics, "Region is unavailable")
	var args core.
		PostOrganizationObjectStorageObjectStorageClusterBucketsJSONRequestBody
	require.NoError(t, json.Unmarshal(requestBody, &args))
	require.Equal(t, "future-region",
		*args.ObjectStorageCluster.Region)
}

func TestObjectStorageBucketReadRemovesMissingRemoteResource(t *testing.T) {
	meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{
				"error": {
					"code": "object_storage_bucket_not_found",
					"description": "not found",
					"detail": {}
				}
			}`))
		},
	))
	r := &ObjectStorageBucketResource{M: meta}
	ctx := context.Background()

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema}
	require.False(t, state.Set(ctx, objectStorageBucketBoundaryModel(
		buildStringSet(nil), buildStringSet(nil),
	)).HasError())
	req := resource.ReadRequest{State: state}
	resp := resource.ReadResponse{State: state}

	r.Read(ctx, req, &resp)

	require.False(t, resp.Diagnostics.HasError())
	require.True(t, resp.State.Raw.IsNull())
}

func TestObjectStorageAccountReadRemovesMissingResourceWithoutPendingTrash(
	t *testing.T,
) {
	meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if req.URL.Path != "/core/v1/organizations/organization/"+
				"object_storage/object_storage_cluster" {
				http.Error(w, "unexpected request", http.StatusInternalServerError)
				return
			}
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

	r.Read(ctx, req, &resp)

	require.False(t, resp.Diagnostics.HasError())
	require.True(t, resp.State.Raw.IsNull())
}

func objectStorageBucketBoundaryModel(
	readKeyIDs types.Set,
	writeKeyIDs types.Set,
) ObjectStorageBucketResourceModel {
	return ObjectStorageBucketResourceModel{
		Name:            types.StringValue("key-removal"),
		Region:          types.StringValue("uk-lon-1"),
		Label:           types.StringNull(),
		PublicURL:       types.StringValue("https://objects.example.test/key-removal"),
		ServeStaticSite: types.BoolValue(false),
		StaticSiteError: types.StringValue(""),
		StaticSiteIndex: types.StringValue(""),
		AllKeysRead:     types.BoolValue(false),
		AllKeysWrite:    types.BoolValue(false),
		PublicList:      types.BoolValue(false),
		PublicRead:      types.BoolValue(false),
		ReadKeyIDs:      readKeyIDs,
		WriteKeyIDs:     writeKeyIDs,
	}
}

func objectStorageBucketPartialPayloadModel() ObjectStorageBucketResourceModel {
	return ObjectStorageBucketResourceModel{
		Name:            types.StringValue("partial-bucket"),
		Region:          types.StringValue("uk-lon-1"),
		Label:           types.StringValue("existing label"),
		PublicURL:       types.StringValue("https://objects.example.test/partial-bucket"),
		ServeStaticSite: types.BoolValue(true),
		StaticSiteError: types.StringValue("error.html"),
		StaticSiteIndex: types.StringValue("index.html"),
		AllKeysRead:     types.BoolValue(true),
		AllKeysWrite:    types.BoolValue(true),
		PublicList:      types.BoolValue(true),
		PublicRead:      types.BoolValue(true),
		ReadKeyIDs:      buildStringSet([]string{"objkey_read"}),
		WriteKeyIDs:     buildStringSet([]string{"objkey_write"}),
	}
}

func objectStorageAccessKeyBoundaryModel() ObjectStorageAccessKeyResourceModel {
	return ObjectStorageAccessKeyResourceModel{
		ID:              types.StringValue("objkey_partial"),
		Name:            types.StringValue("partial-key"),
		Region:          types.StringValue("uk-lon-1"),
		AllBucketsRead:  types.BoolValue(true),
		AllObjectsRead:  types.BoolValue(false),
		AllObjectsWrite: types.BoolValue(true),
		ReadBuckets:     buildStringSet([]string{"read-bucket"}),
		WriteBuckets:    buildStringSet([]string{"write-bucket"}),
		AccessKeyID:     types.StringValue("s3-access-key"),
		SecretAccessKey: types.StringValue("s3-secret-key"),
		ServerURL:       types.StringValue("https://objects.example.test"),
	}
}

func objectStorageReadOperation(
	t *testing.T,
	schemaFunc objectStorageSchemaFunc,
	stateModel any,
) (resource.ReadRequest, resource.ReadResponse) {
	t.Helper()
	ctx := context.Background()
	var schemaResp resource.SchemaResponse
	schemaFunc(ctx, resource.SchemaRequest{}, &schemaResp)

	state := tfsdk.State{Schema: schemaResp.Schema}
	require.False(t, state.Set(ctx, stateModel).HasError())

	return resource.ReadRequest{State: state}, resource.ReadResponse{State: state}
}

func objectStorageUpdateOperation(
	t *testing.T,
	schemaFunc objectStorageSchemaFunc,
	stateModel any,
	planModel any,
) (resource.UpdateRequest, resource.UpdateResponse) {
	t.Helper()
	ctx := context.Background()
	var schemaResp resource.SchemaResponse
	schemaFunc(ctx, resource.SchemaRequest{}, &schemaResp)

	state := tfsdk.State{Schema: schemaResp.Schema}
	require.False(t, state.Set(ctx, stateModel).HasError())
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	require.False(t, plan.Set(ctx, planModel).HasError())

	return resource.UpdateRequest{State: state, Plan: plan},
		resource.UpdateResponse{
			State: tfsdk.State{Schema: schemaResp.Schema, Raw: plan.Raw},
		}
}
