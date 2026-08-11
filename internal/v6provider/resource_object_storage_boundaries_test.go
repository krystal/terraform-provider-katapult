package v6provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
	"github.com/stretchr/testify/require"
)

func TestObjectStorageBucketUpdateSendsEmptyKeySets(t *testing.T) {
	var patchBody []byte
	meta := newObjectStorageFailureTestMeta(t, http.HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch req.Method {
			case http.MethodPatch:
				patchBody, _ = io.ReadAll(req.Body)
				_, _ = w.Write([]byte(`{
					"object_storage_bucket": {"name": "key-removal"}
				}`))
			case http.MethodGet:
				_, _ = w.Write([]byte(`{
					"object_storage_bucket": {
						"name": "key-removal",
						"public_url": "https://objects.example.test/key-removal",
						"serve_static_site": false,
						"access_control_list": {
							"all_keys_read": false,
							"all_keys_write": false,
							"public_list": false,
							"public_read": false,
							"read_key_ids": [],
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
	var args core.PatchObjectStorageObjectStorageClusterBucketJSONRequestBody
	require.NoError(t, json.Unmarshal(patchBody, &args))
	require.NotNil(t, args.Properties.AccessControlList)
	require.NotNil(t, args.Properties.AccessControlList.ReadKeyIds)
	require.Empty(t, *args.Properties.AccessControlList.ReadKeyIds)
	require.NotNil(t, args.Properties.AccessControlList.WriteKeyIds)
	require.Empty(t, *args.Properties.AccessControlList.WriteKeyIds)
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
