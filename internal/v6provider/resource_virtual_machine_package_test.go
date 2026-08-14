package v6provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
)

func TestValidateVirtualMachinePackageChange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		state             string
		packageRef        string
		targetCPUCores    int
		targetMemoryInGB  int
		poweredOn         types.Bool
		wantErr           string
		wantPackageLookup bool
		wantPackageID     bool
	}{
		{
			name:              "running downgrade unmanaged",
			state:             "started",
			packageRef:        "rock-1",
			targetCPUCores:    1,
			targetMemoryInGB:  2,
			poweredOn:         types.BoolNull(),
			wantErr:           "powered_on = false",
			wantPackageLookup: true,
		},
		{
			name:              "running downgrade explicitly off",
			state:             "started",
			packageRef:        "rock-1",
			targetCPUCores:    1,
			targetMemoryInGB:  2,
			poweredOn:         types.BoolValue(false),
			wantPackageLookup: true,
		},
		{
			name:              "running downgrade explicitly on",
			state:             "started",
			packageRef:        "rock-1",
			targetCPUCores:    1,
			targetMemoryInGB:  2,
			poweredOn:         types.BoolValue(true),
			wantErr:           "powered_on = false",
			wantPackageLookup: true,
		},
		{
			name:              "running downgrade unknown power config",
			state:             "started",
			packageRef:        "rock-1",
			targetCPUCores:    1,
			targetMemoryInGB:  2,
			poweredOn:         types.BoolUnknown(),
			wantErr:           "powered_on = false",
			wantPackageLookup: true,
		},
		{
			name:              "starting downgrade explicitly off",
			state:             "starting",
			packageRef:        "rock-1",
			targetCPUCores:    1,
			targetMemoryInGB:  2,
			poweredOn:         types.BoolValue(false),
			wantPackageLookup: true,
		},
		{
			name:              "failed downgrade explicitly off",
			state:             "failed",
			packageRef:        "rock-1",
			targetCPUCores:    1,
			targetMemoryInGB:  2,
			poweredOn:         types.BoolValue(false),
			wantErr:           "failed state",
			wantPackageLookup: true,
		},
		{
			name:              "future state downgrade explicitly off",
			state:             "future",
			packageRef:        "rock-1",
			targetCPUCores:    1,
			targetMemoryInGB:  2,
			poweredOn:         types.BoolValue(false),
			wantErr:           "unsupported state",
			wantPackageLookup: true,
		},
		{
			name:              "running upgrade",
			state:             "started",
			packageRef:        "rock-5",
			targetCPUCores:    4,
			targetMemoryInGB:  8,
			wantPackageLookup: true,
		},
		{
			name:              "failed upgrade",
			state:             "failed",
			packageRef:        "rock-5",
			targetCPUCores:    4,
			targetMemoryInGB:  8,
			wantPackageLookup: true,
		},
		{
			name:       "stopped downgrade unmanaged",
			state:      "stopped",
			packageRef: "rock-1",
			poweredOn:  types.BoolNull(),
		},
		{
			name:       "same package by id",
			state:      "started",
			packageRef: "vmpkg_current",
		},
		{
			name:              "running upgrade by id",
			state:             "started",
			packageRef:        "vmpkg_target",
			targetCPUCores:    4,
			targetMemoryInGB:  8,
			wantPackageLookup: true,
			wantPackageID:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			packageLookups := 0
			server := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")

					switch r.URL.Path {
					case "/virtual_machines/virtual_machine":
						_, _ = w.Write([]byte(`{
							"annotations": [],
							"virtual_machine": {
								"id": "vm_test",
								"state": "` + tt.state + `",
								"package": {
									"id": "vmpkg_current",
									"permalink": "rock-3",
									"cpu_cores": 2,
									"memory_in_gb": 4
								}
							}
						}`))
					case "/virtual_machine_packages/virtual_machine_package":
						packageLookups++
						lookupKey := "virtual_machine_package[permalink]"
						if tt.wantPackageID {
							lookupKey = "virtual_machine_package[id]"
						}
						if got := r.URL.Query().Get(lookupKey); got !=
							tt.packageRef {
							t.Errorf(
								"package lookup = %q, want %q",
								got,
								tt.packageRef,
							)
						}
						_, _ = w.Write([]byte(`{
							"virtual_machine_package": {
								"id": "vmpkg_target",
								"permalink": "` + tt.packageRef + `",
								"cpu_cores": ` +
							strconv.Itoa(tt.targetCPUCores) + `,
								"memory_in_gb": ` +
							strconv.Itoa(tt.targetMemoryInGB) + `
							}
						}`))
					default:
						http.NotFound(w, r)
					}
				},
			))
			defer server.Close()

			client, err := core.NewClientWithResponses(server.URL, "test-token")
			if err != nil {
				t.Fatalf("creating test client: %v", err)
			}

			err = validateVirtualMachinePackageChange(
				context.Background(), client, "vm_test", tt.packageRef,
				tt.poweredOn,
			)
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.wantErr != "" &&
				(err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
			}

			if got := packageLookups > 0; got != tt.wantPackageLookup {
				t.Errorf(
					"package lookup = %t, want %t",
					got,
					tt.wantPackageLookup,
				)
			}
		})
	}
}

func TestChangeVirtualMachinePackage(t *testing.T) {
	t.Parallel()

	var requestBody core.PutVirtualMachinePackageJSONRequestBody
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")

			switch r.URL.Path {
			case "/virtual_machines/virtual_machine":
				_, _ = w.Write([]byte(`{
					"annotations": [],
					"virtual_machine": {
						"id": "vm_test",
						"state": "started",
						"package": {
							"id": "vmpkg_current",
							"permalink": "rock-1",
							"cpu_cores": 1,
							"memory_in_gb": 2
						}
					}
				}`))
			case "/virtual_machines/virtual_machine/package":
				if err := json.NewDecoder(r.Body).Decode(
					&requestBody,
				); err != nil {
					t.Errorf("decoding package request: %v", err)
				}
				_, _ = w.Write([]byte(`{
					"task": {"id": "task_test", "status": "pending"}
				}`))
			case "/tasks/task":
				_, _ = w.Write([]byte(`{
					"task": {"id": "task_test", "status": "completed"}
				}`))
			default:
				http.NotFound(w, r)
			}
		},
	))
	defer server.Close()

	client, err := core.NewClientWithResponses(server.URL, "test-token")
	if err != nil {
		t.Fatalf("creating test client: %v", err)
	}

	m := &Meta{Core: client, testMode: true}
	err = changeVirtualMachinePackage(
		context.Background(), m, "vm_test", "rock-3", time.Second,
	)
	if err != nil {
		t.Fatalf("changing package: %v", err)
	}

	if requestBody.VirtualMachine.Id == nil ||
		*requestBody.VirtualMachine.Id != "vm_test" {
		t.Errorf("virtual machine lookup = %#v", requestBody.VirtualMachine)
	}
	if requestBody.VirtualMachinePackage.Permalink == nil ||
		*requestBody.VirtualMachinePackage.Permalink != "rock-3" {
		t.Errorf("package lookup = %#v", requestBody.VirtualMachinePackage)
	}
}

func TestNormalizeVirtualMachinePackageForState(t *testing.T) {
	t.Parallel()

	id := "vmpkg_test"
	permalink := "rock-3"
	pkg := core.VirtualMachinePackage{Id: &id, Permalink: &permalink}

	tests := []struct {
		name       string
		configured string
		pkg        core.VirtualMachinePackage
		want       string
	}{
		{name: "preserves id", configured: id, pkg: pkg, want: id},
		{
			name:       "preserves permalink",
			configured: permalink,
			pkg:        pkg,
			want:       permalink,
		},
		{name: "import prefers permalink", pkg: pkg, want: permalink},
		{
			name:       "falls back to id",
			configured: permalink,
			pkg:        core.VirtualMachinePackage{Id: &id},
			want:       id,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := normalizeVirtualMachinePackageForState(
				tt.configured, tt.pkg,
			); got != tt.want {
				t.Errorf(
					"normalizeVirtualMachinePackageForState() = %q, want %q",
					got,
					tt.want,
				)
			}
		})
	}
}
