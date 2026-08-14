package v6provider

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiskAssignmentResourceSchema(t *testing.T) {
	t.Parallel()
	r := &DiskAssignmentResource{}
	resp := &frameworkresource.SchemaResponse{}
	r.Schema(context.Background(), frameworkresource.SchemaRequest{}, resp)
	require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics.Errors())

	vmID := resp.Schema.Attributes["virtual_machine_id"].(resourceschema.StringAttribute)
	diskID := resp.Schema.Attributes["disk_id"].(resourceschema.StringAttribute)
	attached := resp.Schema.Attributes["attached"].(resourceschema.BoolAttribute)
	require.True(t, vmID.Required)
	require.True(t, diskID.Required)
	require.NotEmpty(t, vmID.PlanModifiers)
	require.NotEmpty(t, diskID.PlanModifiers)
	require.True(t, attached.Optional)
	require.True(t, attached.Computed)
	require.NotNil(t, attached.Default)
	_, ok := resp.Schema.Blocks["timeouts"].(resourceschema.SingleNestedBlock)
	require.True(t, ok)
}

func TestDiskAssignmentResourceRegisteredOnce(t *testing.T) {
	t.Parallel()
	count := 0
	for _, factory := range (&KatapultProvider{}).Resources(context.Background()) {
		if _, ok := factory().(*DiskAssignmentResource); ok {
			count++
		}
	}
	require.Equal(t, 1, count)
}

func TestParseAssignmentID(t *testing.T) {
	t.Parallel()
	vmID, diskID, err := parseAssignmentID("vm_one/disk_one")
	require.NoError(t, err)
	assert.Equal(t, "vm_one", vmID)
	assert.Equal(t, "disk_one", diskID)
	for _, invalid := range []string{"", "vm", "/disk", "vm/", "vm/disk/extra"} {
		_, _, err = parseAssignmentID(invalid)
		assert.Error(t, err, invalid)
	}
}

//nolint:lll // Compact table rows keep the resize matrix comparable.
func TestEffectiveDiskResizeMethod(t *testing.T) {
	t.Parallel()
	attached := core.VirtualMachineDiskAttachmentStateEnumAttached
	detached := core.VirtualMachineDiskAttachmentStateEnumDetached
	tests := []struct {
		name       string
		oldSize    int64
		newSize    int64
		configured string
		assigned   bool
		state      *core.VirtualMachineDiskAttachmentStateEnum
		want       core.ResizeMethodEnum
		wantErr    bool
	}{
		{name: "offline detached growth", oldSize: 20, newSize: 30, configured: "offline", assigned: true, state: &detached, want: core.Offline},
		{name: "offline attached growth rejected", oldSize: 20, newSize: 30, configured: "offline", assigned: true, state: &attached, wantErr: true},
		{name: "online attached growth", oldSize: 20, newSize: 30, configured: "online", assigned: true, state: &attached, want: core.Online},
		{name: "online detached growth falls back offline", oldSize: 20, newSize: 30, configured: "online", assigned: true, state: &detached, want: core.Offline},
		{name: "detached shrink always offline", oldSize: 30, newSize: 20, configured: "online", assigned: true, state: &detached, want: core.Offline},
		{name: "attached shrink rejected", oldSize: 30, newSize: 20, configured: "online", assigned: true, state: &attached, wantErr: true},
		{name: "unassigned growth offline", oldSize: 20, newSize: 30, configured: "offline", want: core.Offline},
		{name: "unknown assigned state rejected", oldSize: 20, newSize: 30, configured: "online", assigned: true, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			method, err := effectiveDiskResizeMethod(test.oldSize, test.newSize, test.configured, test.assigned, test.state)
			if test.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, method)
		})
	}
}

//nolint:lll // Inline JSON keeps each API error scenario self-contained.
func TestDetachDiskOnlySuppressesUnassignedDisk422(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		body     string
		notFound bool
	}{
		{name: "unassigned", body: `{"error":{"code":"unassigned_disk","description":"not assigned","detail":{}}}`, notFound: true},
		{name: "other validation", body: `{"error":{"code":"validation_error","description":"invalid","detail":{"errors":["busy"]}}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				writeTestJSON(w, http.StatusUnprocessableEntity, test.body)
			})
			err := detachDiskAndWait(context.Background(), &Meta{Core: client, testMode: true}, "disk_test", time.Second)
			if test.notFound {
				assert.ErrorIs(t, err, core.ErrNotFound)
			} else {
				require.Error(t, err)
				assert.False(t, errors.Is(err, core.ErrNotFound))
			}
		})
	}
}

func TestDiskAssignmentLocksSerializePerVM(t *testing.T) {
	t.Parallel()
	m := &Meta{}
	unlock := m.lockDiskAssignments("vm_one")
	acquired := make(chan struct{})
	go func() {
		defer m.lockDiskAssignments("vm_one")()
		close(acquired)
	}()
	select {
	case <-acquired:
		t.Fatal("same-VM lock acquired concurrently")
	case <-time.After(20 * time.Millisecond):
	}
	otherUnlock := m.lockDiskAssignments("vm_two")
	otherUnlock()
	unlock()
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("same-VM waiter did not acquire after unlock")
	}
}

func TestReconcileDiskAssignmentRejectsTransitionalVMBeforeMutation(t *testing.T) {
	t.Parallel()
	patchCalls := 0
	client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/virtual_machines/virtual_machine":
			writeTestJSON(w, http.StatusOK, `{
				"virtual_machine": {"id": "vm_test", "state": "starting"}
			}`)
		case "/disks/disk":
			if req.Method == http.MethodPatch {
				patchCalls++
			}
			writeTestJSON(w, http.StatusOK, `{
				"disk": {
					"id": "disk_test",
					"virtual_machine_disk": {
						"virtual_machine": {"id": "vm_test"},
						"boot": false,
						"attach_on_boot": false,
						"state": "detached"
					}
				}
			}`)
		default:
			http.NotFound(w, req)
		}
	})

	err := reconcileDiskAssignment(
		context.Background(),
		&Meta{Core: client, testMode: true},
		"vm_test",
		"disk_test",
		true,
		time.Second,
	)
	require.ErrorContains(t, err, "transitional state starting")
	assert.Zero(t, patchCalls)
}

func TestWaitForDiskAssignmentAbsentAcceptsUnassignedDisk(t *testing.T) {
	t.Parallel()
	client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/virtual_machines/virtual_machine":
			writeTestJSON(w, http.StatusOK, `{"virtual_machine":{"id":"vm_test","state":"stopped"}}`)
		case "/disks/disk":
			writeTestJSON(w, http.StatusOK, `{"disk":{"id":"disk_test","virtual_machine_disk":null}}`)
		default:
			http.NotFound(w, req)
		}
	})

	err := waitForDiskAssignmentAbsent(
		context.Background(),
		&Meta{Core: client, testMode: true},
		"vm_test",
		"disk_test",
		time.Second,
	)
	require.NoError(t, err)
}

func TestProjectDiskAssignmentAttachedPreservesRepairDiff(t *testing.T) {
	t.Parallel()
	trueValue, falseValue := true, false
	attached := core.VirtualMachineDiskAttachmentStateEnumAttached
	detached := core.VirtualMachineDiskAttachmentStateEnumDetached
	tests := []struct {
		name    string
		desired types.Bool
		state   core.VirtualMachineStateEnum
		policy  *bool
		actual  *core.VirtualMachineDiskAttachmentStateEnum
		want    bool
		wantErr bool
	}{
		{
			name:    "desired true remains visibly false until running attachment converges",
			desired: types.BoolValue(true), state: core.Started,
			policy: &trueValue, actual: &detached, want: false,
		},
		{
			name:    "desired false remains visibly true until physical detach converges",
			desired: types.BoolValue(false), state: core.Started,
			policy: &falseValue, actual: &attached, want: true,
		},
		{
			name:    "stopped desired true converges through attach on boot",
			desired: types.BoolValue(true), state: core.Stopped,
			policy: &trueValue, actual: &detached, want: true,
		},
		{
			name:    "unknown import policy derives the current running observation",
			desired: types.BoolUnknown(), state: core.Started,
			policy: &falseValue, actual: &attached, want: false,
		},
		{
			name:    "unknown VM state fails closed",
			desired: types.BoolValue(false), state: core.VirtualMachineStateEnum(""),
			policy: &falseValue, actual: &detached, wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := projectDiskAssignmentAttached(test.desired, diskAssignmentObservation{
				vmState: test.state, attachOnBoot: test.policy, attachmentState: test.actual,
			})
			if test.wantErr {
				require.ErrorContains(t, err, "unknown state")
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, got.ValueBool())
		})
	}
}
