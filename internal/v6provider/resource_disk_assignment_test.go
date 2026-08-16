package v6provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	resourcetimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
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

func TestDiskAssignmentCreateCheckpointsIdentityBeforeMutationResponse(t *testing.T) {
	t.Parallel()

	client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/virtual_machines/virtual_machine":
			writeTestJSON(w, http.StatusOK, `{"virtual_machine":{"id":"vm_test","state":"stopped"}}`)
		case req.Method == http.MethodGet && req.URL.Path == "/disks/disk":
			writeTestJSON(w, http.StatusOK, `{"disk":{"id":"disk_test","virtual_machine_disk":null}}`)
		case req.Method == http.MethodPost && req.URL.Path == "/disks/disk/assign":
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, req)
		}
	})
	r := &DiskAssignmentResource{M: &Meta{Core: client, testMode: true}}
	plan := diskAssignmentTestState(t, r, DiskAssignmentResourceModel{
		VirtualMachineID: types.StringValue("vm_test"),
		DiskID:           types.StringValue("disk_test"),
		Attached:         types.BoolValue(true),
	})
	resp := frameworkresource.CreateResponse{State: tfsdk.State{Schema: plan.Schema}}

	r.Create(context.Background(), frameworkresource.CreateRequest{Plan: tfsdk.Plan(plan)}, &resp)

	require.True(t, resp.Diagnostics.HasError())
	require.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "unexpected empty response assigning disk")
	var state DiskAssignmentResourceModel
	diags := resp.State.Get(context.Background(), &state)
	require.False(t, diags.HasError(), diags.Errors())
	require.Equal(t, "vm_test/disk_test", state.ID.ValueString())
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
	failed := core.VirtualMachineDiskAttachmentStateEnumFailed
	unknown := core.VirtualMachineDiskAttachmentStateEnum("future_state")
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
		{name: "unrecognized assigned state rejected", oldSize: 20, newSize: 30, configured: "online", assigned: true, state: &unknown, wantErr: true},
		{name: "failed attachment rejected", oldSize: 20, newSize: 30, configured: "online", assigned: true, state: &failed, wantErr: true},
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

func TestDiskAssignmentDeleteRejectsUnsafeAttachmentBeforeMutation(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		state string
		want  string
	}{
		{state: "attaching", want: "transitional or unknown"},
		{state: "failed", want: "repair the failed attachment"},
	} {
		t.Run(test.state, func(t *testing.T) {
			t.Parallel()
			var patchCalls atomic.Int32
			client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, req *http.Request) {
				switch {
				case req.Method == http.MethodGet && req.URL.Path == "/virtual_machines/virtual_machine":
					writeTestJSON(w, http.StatusOK, `{"virtual_machine":{"id":"vm_test","state":"stopped"}}`)
				case req.Method == http.MethodGet && req.URL.Path == "/disks/disk":
					writeTestJSON(w, http.StatusOK, fmt.Sprintf(`{
						"disk":{"id":"disk_test","virtual_machine_disk":{
							"virtual_machine":{"id":"vm_test"},"boot":false,
							"attach_on_boot":true,"state":%q
						}}
					}`, test.state))
				case req.Method == http.MethodPatch && req.URL.Path == "/disks/disk":
					patchCalls.Add(1)
					http.Error(w, "unexpected mutation", http.StatusInternalServerError)
				default:
					http.NotFound(w, req)
				}
			})
			r := &DiskAssignmentResource{M: &Meta{Core: client, testMode: true}}
			state := diskAssignmentTestState(t, r, DiskAssignmentResourceModel{
				ID: types.StringValue("vm_test/disk_test"), VirtualMachineID: types.StringValue("vm_test"),
				DiskID: types.StringValue("disk_test"), Attached: types.BoolValue(true),
				AttachOnBoot: types.BoolValue(true), AttachmentState: types.StringValue(test.state),
			})
			resp := frameworkresource.DeleteResponse{State: state}

			r.Delete(context.Background(), frameworkresource.DeleteRequest{State: state}, &resp)

			require.True(t, resp.Diagnostics.HasError())
			require.Contains(t, resp.Diagnostics.Errors()[0].Detail(), test.want)
			require.Zero(t, patchCalls.Load())
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

func TestWaitForDiskAssignmentConvergencePollsUntilPhysicalStateMatches(t *testing.T) {
	t.Parallel()

	var diskReads atomic.Int32
	client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/virtual_machines/virtual_machine":
			writeTestJSON(w, http.StatusOK, `{"virtual_machine":{"id":"vm_test","state":"started"}}`)
		case "/disks/disk":
			state := "detached"
			if diskReads.Add(1) > 1 {
				state = "attached"
			}
			writeTestJSON(w, http.StatusOK, fmt.Sprintf(`{
				"disk":{"id":"disk_test","virtual_machine_disk":{
					"virtual_machine":{"id":"vm_test"},"boot":false,
					"attach_on_boot":true,"state":%q
				}}
			}`, state))
		default:
			http.NotFound(w, req)
		}
	})

	err := waitForDiskAssignmentConvergence(
		context.Background(), &Meta{Core: client, testMode: true},
		"vm_test", "disk_test", true, time.Second,
	)
	require.NoError(t, err)
	require.GreaterOrEqual(t, diskReads.Load(), int32(2))
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
			name:    "failed VM remains readable through attach on boot policy",
			desired: types.BoolValue(true), state: core.Failed,
			policy: &trueValue, actual: &detached, want: true,
		},
		{
			name:    "orphaned VM remains readable through attach on boot policy",
			desired: types.BoolValue(false), state: core.Orphaned,
			policy: &falseValue, actual: &detached, want: false,
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

func TestDiskAssignmentLifecycleTreatsTrashedVMAsMissing(t *testing.T) {
	t.Parallel()

	for _, operation := range []string{"read", "delete"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			diskRequests := 0
			client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, req *http.Request) {
				switch req.URL.Path {
				case "/virtual_machines/virtual_machine":
					writeObjectInTrashResponse(w)
				case "/disks/disk":
					diskRequests++
					http.Error(w, "unexpected disk request", http.StatusInternalServerError)
				default:
					http.NotFound(w, req)
				}
			})
			r := &DiskAssignmentResource{M: &Meta{Core: client, testMode: true}}
			state := diskAssignmentTestState(t, r, DiskAssignmentResourceModel{
				ID:               types.StringValue("vm_test/disk_test"),
				VirtualMachineID: types.StringValue("vm_test"),
				DiskID:           types.StringValue("disk_test"),
				Attached:         types.BoolValue(true),
				AttachOnBoot:     types.BoolValue(true),
				AttachmentState:  types.StringValue("attached"),
			})

			switch operation {
			case "read":
				resp := frameworkresource.ReadResponse{State: state}
				r.Read(context.Background(), frameworkresource.ReadRequest{State: state}, &resp)
				require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics.Errors())
				require.True(t, resp.State.Raw.IsNull())
			case "delete":
				resp := frameworkresource.DeleteResponse{State: state}
				r.Delete(context.Background(), frameworkresource.DeleteRequest{State: state}, &resp)
				require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics.Errors())
			}
			require.Zero(t, diskRequests)
		})
	}
}

func diskAssignmentTestState(
	t *testing.T,
	r *DiskAssignmentResource,
	model DiskAssignmentResourceModel,
) tfsdk.State {
	t.Helper()
	if len(model.Timeouts.AttributeTypes(context.Background())) == 0 {
		model.Timeouts = resourcetimeouts.Value{Object: types.ObjectNull(
			map[string]attr.Type{
				"create": types.StringType,
				"update": types.StringType,
				"delete": types.StringType,
			},
		)}
	}
	resp := &frameworkresource.SchemaResponse{}
	r.Schema(context.Background(), frameworkresource.SchemaRequest{}, resp)
	require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics.Errors())
	state := tfsdk.State{Schema: resp.Schema}
	diags := state.Set(context.Background(), model)
	require.False(t, diags.HasError(), diags.Errors())
	return state
}
