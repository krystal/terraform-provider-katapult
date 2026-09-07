package v6provider

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/krystal/go-katapult/next/core"
	"github.com/stretchr/testify/require"
)

func TestPurgeTrashObjectVerifiesOriginalResource(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		purgeStatus     int
		trashPolls      int32
		resourceStatus  int
		resourcePending int32
		wantError       string
	}{
		{name: "purged", purgeStatus: 200, resourceStatus: 404},
		{name: "already purged", purgeStatus: 404, resourceStatus: 404},
		{name: "slow purge", purgeStatus: 200, trashPolls: 22, resourceStatus: 404},
		{name: "restored after purge", purgeStatus: 200, resourceStatus: 200, wantError: "still exists outside trash"},
		{name: "restored before purge", purgeStatus: 404, resourceStatus: 200, wantError: "still exists outside trash"},
		{name: "resource still in trash", purgeStatus: 404, resourcePending: 2, resourceStatus: 404},
		{name: "resource lookup denied", purgeStatus: 404, resourceStatus: 403, wantError: "permission_denied"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var trashCalls, resourceCalls, purgeCalls atomic.Int32
			client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, req *http.Request) {
				switch req.Method + " " + req.URL.Path {
				case "DELETE /trash_objects/trash_object":
					purgeCalls.Add(1)
					if tt.purgeStatus == 404 {
						writeTestJSON(w, 404, `{"error":{"code":"trash_object_not_found"}}`)
						return
					}
					writeTestJSON(w, 200, `{}`)
				case "GET /trash_objects/trash_object":
					if resourceCalls.Load() > 0 {
						require.Equal(t, "vm_test", req.URL.Query().Get("trash_object[object_id]"))
						require.Empty(t, req.URL.Query().Get("trash_object[id]"))
					} else {
						require.Equal(t, "trsh_old", req.URL.Query().Get("trash_object[id]"))
					}
					if trashCalls.Add(1) <= tt.trashPolls {
						writeTestJSON(w, 200, `{"trash_object":{"id":"trsh_new"}}`)
						return
					}
					writeTestJSON(w, 404, `{"error":{"code":"trash_object_not_found"}}`)
				case "GET /virtual_machines/virtual_machine":
					require.Equal(t, "vm_test", req.URL.Query().Get("virtual_machine[id]"))
					if resourceCalls.Add(1) <= tt.resourcePending {
						writeObjectInTrashResponse(w)
						return
					}
					switch tt.resourceStatus {
					case 404:
						writeTestJSON(w, 404, `{"error":{"code":"virtual_machine_not_found"}}`)
					case 403:
						writeTestJSON(w, 403, `{"error":{"code":"permission_denied","description":"Not permitted"}}`)
					default:
						writeTestJSON(w, 200, `{"virtual_machine":{"id":"vm_test"}}`)
					}
				default:
					t.Errorf("unexpected request: %s %s", req.Method, req.URL)
					http.Error(w, "unexpected request", 500)
				}
			})
			meta := &Meta{Core: client, testMode: true}
			err := purgeTrashObject(context.Background(), meta, time.Second,
				core.TrashObject{Id: ptr("trsh_old"), ObjectId: ptr("vm_test")},
				virtualMachineDeletionCheck(meta, "vm_test"))
			if tt.wantError != "" {
				require.ErrorContains(t, err, tt.wantError)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, int32(1), purgeCalls.Load())
			require.Equal(t, tt.resourcePending+1, resourceCalls.Load())
			require.Equal(t, tt.trashPolls+tt.resourcePending+1, trashCalls.Load())
		})
	}
}

func TestResourceDeletionChecks(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		endpoint string
		queryKey string
		check    func(*Meta, string) resourceDeletionCheck
	}{
		{"vm", "/virtual_machines/virtual_machine", "virtual_machine[id]", virtualMachineDeletionCheck},
		{"disk", "/disks/disk", "disk[id]", diskDeletionCheck},
		{
			"file storage volume", "/file_storage_volumes/file_storage_volume",
			"file_storage_volume[id]", fileStorageVolumeDeletionCheck,
		},
		{
			"object storage account", "/organizations/organization/object_storage/object_storage_cluster",
			"object_storage_cluster[region]", objectStorageAccountDeletionCheck,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			for _, code := range []int{200, 404, 406, 403} {
				client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, req *http.Request) {
					require.Equal(t, tt.endpoint, req.URL.Path)
					require.Equal(t, "original", req.URL.Query().Get(tt.queryKey))
					if code == 406 {
						writeObjectInTrashResponse(w)
						return
					}
					writeTestJSON(w, code, `{"error":{"code":"permission_denied","description":"Not permitted"}}`)
				})
				deleted, err := tt.check(&Meta{Core: client}, "original")(context.Background())
				switch code {
				case 404:
					require.NoError(t, err)
					require.True(t, deleted)
				case 406:
					require.NoError(t, err)
					require.False(t, deleted)
				case 200:
					require.ErrorContains(t, err, "still exists outside trash")
					require.False(t, deleted)
				case 403:
					require.ErrorContains(t, err, "permission_denied")
					require.False(t, deleted)
				}
			}
		})
	}
}

func TestWaitForTrashObjectNotFoundHonorsTimeout(t *testing.T) {
	client := &pollingCoreClient{getTrashObject: func(
		context.Context, *core.GetTrashObjectParams, ...core.RequestEditorFn,
	) (*core.GetTrashObjectResponse, error) {
		return &core.GetTrashObjectResponse{}, core.ErrNotFound
	}}
	err := waitForTrashObjectNotFound(
		context.Background(), &Meta{Core: client, testMode: true}, 30*time.Millisecond, core.TrashObject{},
		func(context.Context) (bool, error) { return false, nil })
	require.ErrorContains(t, err, "timeout while waiting")
	require.NotContains(t, err.Error(), "couldn't find resource")
}
