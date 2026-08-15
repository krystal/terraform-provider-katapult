package v6provider

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	core "github.com/krystal/go-katapult/next/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiskDataSourceModelCompleteAndNull(t *testing.T) {
	t.Parallel()

	disk := core.GetDisk200ResponseDisk{
		Id:           ptr("disk_test"),
		Name:         ptr("Data"),
		SizeInGb:     ptr(20),
		StorageSpeed: ptr(core.Nvme),
		Wwn:          ptr("wwn_test"),
		State:        ptr(core.DiskStateEnumBuilt),
		DataCenter: &core.GetDiskPartDataCenter{
			Id: ptr("dc_test"), Name: ptr("Test"), Permalink: ptr("test-1"),
		},
	}
	disk.BusType.Set(core.Virtio)
	disk.IoProfile.Set(core.DiskIOProfile{Id: ptr("iop_test")})
	disk.VirtualMachineDisk.Set(core.GetDiskPartVirtualMachineDisk{
		AttachOnBoot: ptr(true),
		Boot:         ptr(false),
		State:        ptr(core.VirtualMachineDiskAttachmentStateEnumAttached),
		VirtualMachine: &core.GetDiskPartVirtualMachine{
			Id:   ptr("vm_test"),
			Fqdn: ptr("vm.example.test"),
		},
	})

	model := diskDataSourceModel(&disk)
	assert.Equal(t, types.StringValue("disk_test"), model.ID)
	assert.Equal(t, types.StringValue("Data"), model.Name)
	assert.Equal(t, types.Int64Value(20), model.SizeInGB)
	assert.Equal(t, types.StringValue("nvme"), model.StorageSpeed)
	assert.Equal(t, types.StringValue("virtio"), model.BusType)
	assert.Equal(t, types.StringValue("iop_test"), model.IOProfileID)
	assert.Equal(t, types.StringValue("wwn_test"), model.WWN)
	assert.Equal(t, types.StringValue("built"), model.State)
	assert.Equal(t, types.StringValue("dc_test"), model.DataCenterID)
	assert.Equal(t, types.StringValue("Test"), model.DataCenterName)
	assert.Equal(t, types.StringValue("test-1"), model.DataCenterPermalink)
	assert.Equal(t, types.StringValue("vm_test"), model.VirtualMachineID)
	assert.Equal(t, types.StringValue("vm.example.test"), model.VirtualMachineFQDN)
	assert.Equal(t, types.BoolValue(false), model.Boot)
	assert.Equal(t, types.BoolValue(true), model.AttachOnBoot)
	assert.Equal(t, types.StringValue("attached"), model.AttachmentState)

	nullDisk := core.GetDisk200ResponseDisk{}
	nullDisk.BusType.SetNull()
	nullDisk.IoProfile.SetNull()
	nullDisk.VirtualMachineDisk.SetNull()
	nullModel := diskDataSourceModel(&nullDisk)
	assert.True(t, nullModel.ID.IsNull())
	assert.True(t, nullModel.SizeInGB.IsNull())
	assert.True(t, nullModel.StorageSpeed.IsNull())
	assert.True(t, nullModel.BusType.IsNull())
	assert.True(t, nullModel.IOProfileID.IsNull())
	assert.True(t, nullModel.DataCenterID.IsNull())
	assert.True(t, nullModel.VirtualMachineID.IsNull())
	assert.True(t, nullModel.Boot.IsNull())
	assert.True(t, nullModel.AttachOnBoot.IsNull())
	assert.True(t, nullModel.AttachmentState.IsNull())
}

func TestDiskSummaryDataSourceModelsAssignmentAndSorting(t *testing.T) {
	t.Parallel()

	assigned := core.GetOrganizationDisks200ResponseDisk{Id: ptr("disk_a")}
	assigned.VirtualMachineDisk.Set(
		core.GetOrganizationDisksPartVirtualMachineDisk{
			VirtualMachine: &core.GetOrganizationDisksPartVirtualMachine{
				Id: ptr("vm_test"), Fqdn: ptr("vm.example.test"),
			},
		},
	)
	unassigned := core.GetOrganizationDisks200ResponseDisk{Id: ptr("disk_z")}
	unassigned.VirtualMachineDisk.SetNull()
	disks := []core.GetOrganizationDisks200ResponseDisk{
		unassigned,
		{Id: nil},
		assigned,
	}

	models := diskSummaryDataSourceModels(disks)
	require.Len(t, models, 3)
	assert.Equal(t, "disk_a", models[0].ID.ValueString())
	assert.Equal(t, "vm_test", models[0].VirtualMachineID.ValueString())
	assert.Equal(t, "vm.example.test", models[0].VirtualMachineFQDN.ValueString())
	assert.Equal(t, "disk_z", models[1].ID.ValueString())
	assert.True(t, models[1].VirtualMachineID.IsNull())
	assert.True(t, models[2].ID.IsNull())
}

func TestFetchAllOrganizationDisksPaginationEmptyErrorAndNoFanOut(t *testing.T) {
	t.Parallel()

	t.Run("pagination and request shape", func(t *testing.T) {
		t.Parallel()
		var requests atomic.Int32
		client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			require.Equal(t, "/organizations/organization/disks", r.URL.Path)
			require.Equal(t, "test-org", r.URL.Query().Get("organization[sub_domain]"))
			require.Equal(t, "200", r.URL.Query().Get("per_page"))
			switch r.URL.Query().Get("page") {
			case "1":
				writeTestJSON(w, http.StatusOK, `{"disk":[{"id":"disk_z"}],"pagination":{"total_pages":2}}`)
			case "2":
				writeTestJSON(w, http.StatusOK, `{"disk":[{"id":"disk_a"}],"pagination":{"total_pages":2}}`)
			default:
				t.Errorf("unexpected page %q", r.URL.Query().Get("page"))
			}
		})

		disks, err := fetchAllOrganizationDisks(context.Background(), &Meta{
			Core: client, confOrganization: "test-org",
		})
		require.NoError(t, err)
		require.Len(t, disks, 2)
		assert.Equal(t, int32(2), requests.Load())
		models := diskSummaryDataSourceModels(disks)
		assert.Equal(t, "disk_a", models[0].ID.ValueString())
		assert.Equal(t, "disk_z", models[1].ID.ValueString())
	})

	t.Run("known empty", func(t *testing.T) {
		t.Parallel()
		client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			writeTestJSON(w, http.StatusOK, `{"disk":[],"pagination":{"total_pages":1}}`)
		})
		disks, err := fetchAllOrganizationDisks(context.Background(), &Meta{Core: client})
		require.NoError(t, err)
		assert.NotNil(t, disks)
		assert.Empty(t, disks)
	})

	for _, test := range []struct {
		name        string
		contentType string
		status      int
		body        string
		want        string
	}{
		{
			name: "API error", contentType: "application/json",
			status: http.StatusInternalServerError,
			body:   `{"error":{"code":"broken","description":"disk listing failed"}}`,
			want:   "broken: disk listing failed",
		},
		{
			name: "empty successful response", contentType: "text/plain",
			status: http.StatusOK, want: "unexpected empty response",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", test.contentType)
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			})
			_, err := fetchAllOrganizationDisks(context.Background(), &Meta{Core: client})
			require.ErrorContains(t, err, test.want)
		})
	}
}

func TestPaginationHasNext(t *testing.T) {
	t.Parallel()

	pagination := core.PaginationObject{}
	pagination.TotalPages.Set(2)
	assert.True(t, paginationHasNext(pagination, 1, 1))
	assert.False(t, paginationHasNext(pagination, 2, 200))
	assert.True(t, paginationHasNext(core.PaginationObject{}, 1, 200))
	assert.False(t, paginationHasNext(core.PaginationObject{}, 1, 199))
}
