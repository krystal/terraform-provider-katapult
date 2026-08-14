package v6provider

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
	"github.com/stretchr/testify/require"
)

func TestVirtualMachinePoweredOnProjection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		state    core.VirtualMachineStateEnum
		previous types.Bool
		want     types.Bool
	}{
		{name: "started", state: core.Started, want: types.BoolValue(true)},
		{name: "starting", state: core.Starting, want: types.BoolValue(true)},
		{name: "stopped", state: core.Stopped, want: types.BoolValue(false)},
		{name: "stopping", state: core.Stopping, want: types.BoolValue(false)},
		{
			name:  "shutting down",
			state: core.ShuttingDown,
			want:  types.BoolValue(false),
		},
		{
			name:     "migrating preserves known value",
			state:    core.Migrating,
			previous: types.BoolValue(true),
			want:     types.BoolValue(true),
		},
		{
			name:  "failed remains unknown without previous value",
			state: core.Failed,
			want:  types.BoolNull(),
		},
		{
			name:     "future state preserves known value",
			state:    core.VirtualMachineStateEnum("future"),
			previous: types.BoolValue(false),
			want:     types.BoolValue(false),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := virtualMachinePoweredOnProjection(tt.state, tt.previous)
			require.True(t, got.Equal(tt.want), "got %s, want %s", got, tt.want)
		})
	}
}

func TestVirtualMachinePowerReconciliation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		poweredOn    bool
		states       []core.VirtualMachineStateEnum
		wantAction   string
		wantEndpoint string
	}{
		{
			name:      "started target on is a no-op",
			poweredOn: true,
			states:    []core.VirtualMachineStateEnum{core.Started},
		},
		{
			name:         "started target off queues graceful shutdown",
			states:       []core.VirtualMachineStateEnum{core.Started, core.Stopped},
			wantAction:   "shutdown",
			wantEndpoint: "/virtual_machines/virtual_machine/shutdown",
		},
		{
			name:         "stopped target on queues start",
			poweredOn:    true,
			states:       []core.VirtualMachineStateEnum{core.Stopped, core.Started},
			wantAction:   "start",
			wantEndpoint: "/virtual_machines/virtual_machine/start",
		},
		{
			name:      "starting target on waits without duplicate action",
			poweredOn: true,
			states:    []core.VirtualMachineStateEnum{core.Starting, core.Started},
		},
		{
			name:      "starting target on reverts then starts once",
			poweredOn: true,
			states: []core.VirtualMachineStateEnum{
				core.Starting, core.Stopped, core.Stopped, core.Started,
			},
			wantAction:   "start",
			wantEndpoint: "/virtual_machines/virtual_machine/start",
		},
		{
			name:   "shutting down target off waits without duplicate action",
			states: []core.VirtualMachineStateEnum{core.ShuttingDown, core.Stopped},
		},
		{
			name: "shutting down target off reverts then shuts down once",
			states: []core.VirtualMachineStateEnum{
				core.ShuttingDown, core.Started, core.Started, core.Stopped,
			},
			wantAction:   "shutdown",
			wantEndpoint: "/virtual_machines/virtual_machine/shutdown",
		},
		{
			name:      "opposite transition settles before shutdown",
			poweredOn: false,
			states: []core.VirtualMachineStateEnum{
				core.Starting, core.Started, core.Started, core.Stopped,
			},
			wantAction:   "shutdown",
			wantEndpoint: "/virtual_machines/virtual_machine/shutdown",
		},
		{
			name:      "starting can revert to stopped target without action",
			poweredOn: false,
			states: []core.VirtualMachineStateEnum{
				core.Starting, core.Stopped, core.Stopped,
			},
		},
		{
			name:      "shutting down can revert to started target without action",
			poweredOn: true,
			states: []core.VirtualMachineStateEnum{
				core.ShuttingDown, core.Started, core.Started,
			},
		},
		{
			name:      "ambiguous transition settles before start",
			poweredOn: true,
			states: []core.VirtualMachineStateEnum{
				core.Migrating, core.Stopped, core.Stopped, core.Started,
			},
			wantAction:   "start",
			wantEndpoint: "/virtual_machines/virtual_machine/start",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stateCall := 0
			actionCalls := 0
			var gotEndpoint string
			client := newVirtualMachineTestClient(t, func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				switch {
				case r.Method == http.MethodGet &&
					r.URL.Path == "/virtual_machines/virtual_machine":
					index := stateCall
					if index >= len(tt.states) {
						index = len(tt.states) - 1
					}
					stateCall++
					writeVirtualMachinePowerState(w, "vm_power", tt.states[index])
				case r.Method == http.MethodPost &&
					strings.HasPrefix(
						r.URL.Path, "/virtual_machines/virtual_machine/",
					):
					actionCalls++
					gotEndpoint = r.URL.Path
					writeTestJSON(w, http.StatusOK, `{
						"task": {"id": "task_power", "status": "pending"}
					}`)
				case r.Method == http.MethodGet &&
					r.URL.Path == "/tasks/task":
					writeTestJSON(w, http.StatusOK, `{
						"task": {"id": "task_power", "status": "completed"}
					}`)
				default:
					http.NotFound(w, r)
				}
			})
			meta := &Meta{Core: client, testMode: true}

			err := reconcileVirtualMachinePowerState(
				context.Background(), meta, "vm_power", tt.poweredOn, time.Second,
			)

			require.NoError(t, err)
			if tt.wantAction == "" {
				require.Zero(t, actionCalls)
			} else {
				require.Equal(t, 1, actionCalls)
				require.Equal(t, tt.wantEndpoint, gotEndpoint)
			}
		})
	}
}

func TestVirtualMachinePowerReconciliationRejectsUnsafeStates(t *testing.T) {
	t.Parallel()

	for _, state := range []core.VirtualMachineStateEnum{
		core.Failed,
		core.Orphaned,
		core.VirtualMachineStateEnum("future"),
	} {
		t.Run(string(state), func(t *testing.T) {
			t.Parallel()

			actionCalls := 0
			client := newVirtualMachineTestClient(t, func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				if r.Method == http.MethodGet {
					writeVirtualMachinePowerState(w, "vm_power", state)
					return
				}
				actionCalls++
				http.NotFound(w, r)
			})

			err := reconcileVirtualMachinePowerState(
				context.Background(),
				&Meta{Core: client, testMode: true},
				"vm_unsafe",
				true,
				time.Second,
			)

			require.Error(t, err)
			require.Contains(t, err.Error(), string(state))
			require.Zero(t, actionCalls)
		})
	}
}

func TestVirtualMachinePowerReconciliationActionFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		actionBody string
		actionCode int
		taskBody   string
		wantErr    string
	}{
		{
			name: "structured action error",
			actionBody: `{
				"error": {"code": "permission_denied", "description": "No access"}
			}`,
			actionCode: http.StatusForbidden,
			wantErr:    "permission_denied: No access",
		},
		{
			name:       "missing task id",
			actionBody: `{"task": {"status": "pending"}}`,
			actionCode: http.StatusOK,
			wantErr:    "unexpected empty task response",
		},
		{
			name:       "failed task",
			actionBody: `{"task": {"id": "task_failed", "status": "pending"}}`,
			actionCode: http.StatusOK,
			taskBody:   `{"task": {"id": "task_failed", "status": "failed"}}`,
			wantErr:    "shutdown task: task failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newVirtualMachineTestClient(t, func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				switch r.URL.Path {
				case "/virtual_machines/virtual_machine":
					writeVirtualMachinePowerState(w, "vm_power", core.Started)
				case "/virtual_machines/virtual_machine/shutdown":
					writeTestJSON(w, tt.actionCode, tt.actionBody)
				case "/tasks/task":
					writeTestJSON(w, http.StatusOK, tt.taskBody)
				default:
					http.NotFound(w, r)
				}
			})

			err := reconcileVirtualMachinePowerState(
				context.Background(),
				&Meta{Core: client, testMode: true},
				"vm_failure",
				false,
				time.Second,
			)

			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func writeVirtualMachinePowerState(
	w http.ResponseWriter,
	id string,
	state core.VirtualMachineStateEnum,
) {
	writeTestJSON(w, http.StatusOK, fmt.Sprintf(`{
		"annotations": [],
		"virtual_machine": {"id": %q, "state": %q}
	}`, id, state))
}
