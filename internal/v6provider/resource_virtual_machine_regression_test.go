package v6provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	resourcetimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	frameworkdatasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	frameworkvalidator "github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/krystal/go-katapult/next/core"
	"github.com/stretchr/testify/require"
)

func TestVirtualMachineResourceReadRemovesMissingResource(t *testing.T) {
	t.Parallel()

	client := newVirtualMachineTestClient(t, func(
		w http.ResponseWriter,
		_ *http.Request,
	) {
		writeTestJSON(w, http.StatusNotFound, `{
			"error": {
				"code": "virtual_machine_not_found",
				"description": "No virtual machine was found"
			}
		}`)
	})
	resource := &VirtualMachineResource{M: &Meta{Core: client, testMode: true}}
	state := virtualMachineTestState(t, resource, VirtualMachineResourceModel{
		ID: types.StringValue("vm_missing"),
	})

	req := frameworkresource.ReadRequest{State: state}
	resp := frameworkresource.ReadResponse{State: state}
	resource.Read(context.Background(), req, &resp)

	require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics.Errors())
	require.True(
		t, resp.State.Raw.IsNull(), "missing VM should be removed from state",
	)
}

func TestVirtualMachineResourceReadRemovesTrashedResource(t *testing.T) {
	t.Parallel()

	client := newVirtualMachineTestClient(t, func(
		w http.ResponseWriter,
		_ *http.Request,
	) {
		writeObjectInTrashResponse(w)
	})
	resource := &VirtualMachineResource{M: &Meta{Core: client, testMode: true}}
	state := virtualMachineTestState(t, resource, VirtualMachineResourceModel{
		ID: types.StringValue("vm_trashed"),
	})

	req := frameworkresource.ReadRequest{State: state}
	resp := frameworkresource.ReadResponse{State: state}
	resource.Read(context.Background(), req, &resp)

	require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics.Errors())
	require.True(
		t, resp.State.Raw.IsNull(), "trashed VM should be removed from state",
	)
}

func TestVirtualMachineGroupResourceReadRemovesMissingResource(t *testing.T) {
	t.Parallel()

	client := newVirtualMachineTestClient(t, func(
		w http.ResponseWriter,
		_ *http.Request,
	) {
		writeTestJSON(w, http.StatusNotFound, `{
			"error": {
				"code": "virtual_machine_group_not_found",
				"description": "No virtual machine group was found"
			}
		}`)
	})
	resource := &VirtualMachineGroupResource{
		M: &Meta{Core: client, testMode: true},
	}
	state := virtualMachineGroupTestState(
		t,
		resource,
		VirtualMachineGroupResourceModel{
			ID: types.StringValue("vmgrp_missing"),
		},
	)

	req := frameworkresource.ReadRequest{State: state}
	resp := frameworkresource.ReadResponse{State: state}
	resource.Read(context.Background(), req, &resp)

	require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics.Errors())
	require.True(
		t,
		resp.State.Raw.IsNull(),
		"missing VM group should be removed from state",
	)
}

func TestVirtualMachineResourceDeleteAlreadyInTrash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		skipPurge   bool
		purgeStatus int
		wantPurge   int
	}{
		{name: "skip purge", skipPurge: true},
		{name: "purge", purgeStatus: http.StatusOK, wantPurge: 1},
		{
			name:        "purge entry disappeared",
			purgeStatus: http.StatusNotFound,
			wantPurge:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			purgeCalls := 0
			client := newVirtualMachineTestClient(t, func(
				w http.ResponseWriter,
				r *http.Request,
			) {
				switch {
				case r.Method == http.MethodGet &&
					r.URL.Path == "/virtual_machines/virtual_machine/disks":
					writeObjectInTrashResponse(w)
				case r.Method == http.MethodGet &&
					r.URL.Path == "/virtual_machines/virtual_machine":
					writeObjectInTrashResponse(w)
				case r.Method == http.MethodDelete &&
					r.URL.Path == "/trash_objects/trash_object":
					purgeCalls++
					writeTestJSON(w, tt.purgeStatus, `{}`)
				case r.Method == http.MethodGet &&
					r.URL.Path == "/trash_objects/trash_object":
					writeTestJSON(w, http.StatusNotFound, `{
						"error": {"code": "trash_object_not_found"}
					}`)
				default:
					http.NotFound(w, r)
				}
			})
			resource := &VirtualMachineResource{M: &Meta{
				Core:                 client,
				SkipTrashObjectPurge: tt.skipPurge,
				testMode:             true,
			}}
			state := virtualMachineTestState(
				t,
				resource,
				VirtualMachineResourceModel{
					ID:           types.StringValue("vm_trashed"),
					IPAddressIDs: types.SetValueMust(types.StringType, nil),
				},
			)

			req := frameworkresource.DeleteRequest{State: state}
			resp := frameworkresource.DeleteResponse{State: state}
			resource.Delete(context.Background(), req, &resp)

			require.False(
				t, resp.Diagnostics.HasError(), resp.Diagnostics.Errors(),
			)
			require.Equal(t, tt.wantPurge, purgeCalls)
		})
	}
}

func TestVirtualMachineResourceDeleteRacePurgesByObjectID(t *testing.T) {
	t.Parallel()

	purgeCalls := 0
	deleteCalls := 0
	var purgeBody core.DeleteTrashObjectJSONRequestBody
	var purgeBodyErr error
	client := newVirtualMachineTestClient(t, func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		switch {
		case r.Method == http.MethodGet &&
			r.URL.Path == "/virtual_machines/virtual_machine":
			writeTestJSON(w, http.StatusOK, `{
				"annotations": [],
				"virtual_machine": {
					"id": "vm_race",
					"hostname": "vm-race",
					"state": "stopped"
				}
			}`)
		case r.Method == http.MethodDelete &&
			r.URL.Path == "/virtual_machines/virtual_machine":
			deleteCalls++
			writeObjectInTrashResponse(w)
		case r.Method == http.MethodDelete &&
			r.URL.Path == "/trash_objects/trash_object":
			purgeCalls++
			purgeBodyErr = json.NewDecoder(r.Body).Decode(&purgeBody)
			writeTestJSON(w, http.StatusOK, `{}`)
		case r.Method == http.MethodGet &&
			r.URL.Path == "/trash_objects/trash_object":
			writeTestJSON(w, http.StatusNotFound, `{
				"error": {"code": "trash_object_not_found"}
			}`)
		default:
			http.NotFound(w, r)
		}
	})
	resource := &VirtualMachineResource{M: &Meta{Core: client, testMode: true}}
	state := virtualMachineTestState(t, resource, VirtualMachineResourceModel{
		ID:           types.StringValue("vm_race"),
		IPAddressIDs: types.SetValueMust(types.StringType, nil),
	})

	req := frameworkresource.DeleteRequest{State: state}
	resp := frameworkresource.DeleteResponse{State: state}
	resource.Delete(context.Background(), req, &resp)

	require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics.Errors())
	require.Equal(t, 1, deleteCalls)
	require.Equal(t, 1, purgeCalls)
	require.NoError(t, purgeBodyErr)
	require.Nil(t, purgeBody.TrashObject.Id)
	require.NotNil(t, purgeBody.TrashObject.ObjectId)
	require.Equal(t, "vm_race", *purgeBody.TrashObject.ObjectId)
}

func TestVirtualMachineDeleteRejectsRemainingNonBootAssignmentBeforeMutation(
	t *testing.T,
) {
	t.Parallel()

	var deleteRequests atomic.Int32
	client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet &&
			r.URL.Path == "/virtual_machines/virtual_machine/disks":
			writeTestJSON(w, http.StatusOK, `{
				"disks":[
					{"disk":{"id":"disk_boot"},"boot":true,"attach_on_boot":true,"state":"attached"},
					{"disk":{"id":"disk_data"},"boot":false,"attach_on_boot":true,"state":"attached"}
				],
				"pagination":{"total_pages":1}
			}`)
		case r.Method == http.MethodDelete:
			deleteRequests.Add(1)
			http.Error(w, "unexpected delete", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	})
	r := &VirtualMachineResource{M: &Meta{Core: client, testMode: true}}
	state := virtualMachineTestState(t, r, VirtualMachineResourceModel{
		ID:         types.StringValue("vm_test"),
		SystemDisk: knownTestSystemDisk(t, "disk_boot"),
	})
	resp := frameworkresource.DeleteResponse{State: state}

	r.Delete(
		context.Background(), frameworkresource.DeleteRequest{State: state}, &resp,
	)
	require.True(t, resp.Diagnostics.HasError())
	require.Contains(
		t,
		resp.Diagnostics.Errors()[0].Detail(),
		"still has non-boot disk relationships",
	)
	require.Zero(t, deleteRequests.Load())
}

func TestVirtualMachineResourceUpdateClearsDescriptionWithoutUnknownName(
	t *testing.T,
) {
	t.Parallel()

	patchCalls := 0
	var patchBody vmGroupPatchBody
	var patchBodyErr error
	client := newVirtualMachineTestClient(t, func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		if writeVirtualMachineBootDiskTestResponse(w, r, "disk_vm_test") {
			return
		}
		switch {
		case r.Method == http.MethodPatch &&
			r.URL.Path == "/virtual_machines/virtual_machine":
			patchCalls++
			patchBodyErr = json.NewDecoder(r.Body).Decode(&patchBody)
			writeTestJSON(w, http.StatusOK, `{
				"virtual_machine": {"id": "vm_test"}
			}`)
		case r.Method == http.MethodGet &&
			r.URL.Path == "/virtual_machines/virtual_machine":
			writeTestJSON(w, http.StatusOK, `{
				"annotations": [],
				"virtual_machine": {
					"id": "vm_test",
					"name": "Stable name",
					"hostname": "stable-hostname",
					"fqdn": "stable-hostname.example.test",
					"state": "stopped",
					"package": {
						"id": "vmpkg_test",
						"permalink": "rock-3"
					},
					"ip_addresses": [],
					"tag_names": []
				}
			}`)
		case r.Method == http.MethodGet &&
			r.URL.Path ==
				"/virtual_machines/virtual_machine/network_interfaces":
			writeTestJSON(w, http.StatusOK, `{
				"pagination": {"total_pages": 1},
				"virtual_machine_network_interfaces": []
			}`)
		default:
			http.NotFound(w, r)
		}
	})
	resource := &VirtualMachineResource{M: &Meta{Core: client, testMode: true}}

	stateModel := VirtualMachineResourceModel{
		ID:           types.StringValue("vm_test"),
		Name:         types.StringValue("Stable name"),
		Hostname:     types.StringValue("stable-hostname"),
		Description:  types.StringValue("Old description"),
		Package:      types.StringValue("rock-3"),
		DiskTemplate: types.StringValue("ubuntu-18-04"),
		SystemDisk:   knownTestSystemDisk(t, "disk_vm_test"),
	}
	planModel := stateModel
	planModel.Name = types.StringUnknown()
	planModel.Description = types.StringNull()
	configModel := planModel
	configModel.Name = types.StringNull()

	state := virtualMachineTestState(t, resource, stateModel)
	planState := virtualMachineTestState(t, resource, planModel)
	configState := virtualMachineTestState(t, resource, configModel)
	req := frameworkresource.UpdateRequest{
		Config: tfsdk.Config(configState),
		Plan:   tfsdk.Plan(planState),
		State:  state,
	}
	resp := frameworkresource.UpdateResponse{State: tfsdk.State{
		Schema: state.Schema,
	}}

	resource.Update(context.Background(), req, &resp)

	require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics.Errors())
	require.Equal(t, 1, patchCalls)
	require.NoError(t, patchBodyErr)
	require.Nil(t, patchBody.Properties.Name)
	require.NotNil(t, patchBody.Properties.Description)
	require.Equal(t, "", *patchBody.Properties.Description)
}

func TestVirtualMachineResourceUpdateShutsDownBeforePackageDowngrade(
	t *testing.T,
) {
	t.Parallel()

	var operations []string
	shutdownRequested := false
	packageChanged := false
	client := newVirtualMachineTestClient(t, func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		if writeVirtualMachineBootDiskTestResponse(w, r, "disk_vm_downgrade") {
			return
		}
		switch {
		case r.Method == http.MethodGet &&
			r.URL.Path == "/virtual_machines/virtual_machine":
			state := "started"
			pkg := "rock-3"
			if shutdownRequested {
				state = "stopped"
			}
			if packageChanged {
				pkg = "rock-1"
			}
			writeTestJSON(w, http.StatusOK, `{
				"annotations": [],
				"virtual_machine": {
					"id": "vm_downgrade",
					"name": "Downgrade VM",
					"hostname": "downgrade-vm",
					"fqdn": "downgrade-vm.example.test",
					"state": "`+state+`",
					"package": {
						"id": "vmpkg_test",
						"permalink": "`+pkg+`"
					},
					"ip_addresses": [],
					"tag_names": []
				}
			}`)
		case r.Method == http.MethodPost &&
			r.URL.Path == "/virtual_machines/virtual_machine/shutdown":
			operations = append(operations, "shutdown")
			shutdownRequested = true
			writeTestJSON(w, http.StatusOK, `{
				"task": {"id": "task_shutdown", "status": "pending"}
			}`)
		case r.Method == http.MethodPut &&
			r.URL.Path == "/virtual_machines/virtual_machine/package":
			operations = append(operations, "package")
			packageChanged = true
			writeTestJSON(w, http.StatusOK, `{
				"task": {"id": "task_package", "status": "pending"}
			}`)
		case r.Method == http.MethodGet && r.URL.Path == "/tasks/task":
			writeTestJSON(w, http.StatusOK, `{
				"task": {"id": "task", "status": "completed"}
			}`)
		case r.Method == http.MethodGet &&
			r.URL.Path ==
				"/virtual_machines/virtual_machine/network_interfaces":
			writeTestJSON(w, http.StatusOK, `{
				"pagination": {"total_pages": 1},
				"virtual_machine_network_interfaces": []
			}`)
		default:
			http.NotFound(w, r)
		}
	})
	resource := &VirtualMachineResource{M: &Meta{Core: client, testMode: true}}
	emptyStrings := types.SetValueMust(types.StringType, nil)
	stateModel := VirtualMachineResourceModel{
		ID:                types.StringValue("vm_downgrade"),
		Name:              types.StringValue("Downgrade VM"),
		Hostname:          types.StringValue("downgrade-vm"),
		FQDN:              types.StringValue("downgrade-vm.example.test"),
		State:             types.StringValue("started"),
		PoweredOn:         types.BoolValue(true),
		Package:           types.StringValue("rock-3"),
		DiskTemplate:      types.StringValue("ubuntu-18-04"),
		SystemDisk:        knownTestSystemDisk(t, "disk_vm_downgrade"),
		IPAddressIDs:      emptyStrings,
		IPAddresses:       emptyStrings,
		VirtualNetworkIDs: emptyStrings,
		Tags:              emptyStrings,
	}
	planModel := stateModel
	planModel.State = types.StringUnknown()
	planModel.PoweredOn = types.BoolValue(false)
	planModel.Package = types.StringValue("rock-1")

	state := virtualMachineTestState(t, resource, stateModel)
	planState := virtualMachineTestState(t, resource, planModel)
	req := frameworkresource.UpdateRequest{
		Config: tfsdk.Config(planState),
		Plan:   tfsdk.Plan(planState),
		State:  state,
	}
	resp := frameworkresource.UpdateResponse{State: tfsdk.State{
		Schema: state.Schema,
	}}

	resource.Update(context.Background(), req, &resp)

	require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics.Errors())
	require.Equal(t, []string{"shutdown", "package"}, operations)
	var got VirtualMachineResourceModel
	diags := resp.State.Get(context.Background(), &got)
	require.False(t, diags.HasError(), diags.Errors())
	require.Equal(t, "rock-1", got.Package.ValueString())
	require.Equal(t, "stopped", got.State.ValueString())
	require.False(t, got.PoweredOn.ValueBool())
}

func TestVirtualMachineResourceUpdatePoweredOnNoDriftQueuesNoPowerAction(
	t *testing.T,
) {
	t.Parallel()

	patchCalls := 0
	powerCalls := 0
	client := newVirtualMachineTestClient(t, func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		if writeVirtualMachineBootDiskTestResponse(w, r, "disk_vm_started") {
			return
		}
		switch {
		case r.Method == http.MethodPatch &&
			r.URL.Path == "/virtual_machines/virtual_machine":
			patchCalls++
			writeTestJSON(w, http.StatusOK, `{
				"virtual_machine": {"id": "vm_started"}
			}`)
		case r.Method == http.MethodPost &&
			strings.HasPrefix(
				r.URL.Path, "/virtual_machines/virtual_machine/",
			):
			powerCalls++
			http.Error(w, "unexpected power action", http.StatusInternalServerError)
		case r.Method == http.MethodGet &&
			r.URL.Path == "/virtual_machines/virtual_machine":
			writeTestJSON(w, http.StatusOK, `{
				"annotations": [],
				"virtual_machine": {
					"id": "vm_started",
					"name": "Started VM",
					"hostname": "started-vm",
					"description": "New description",
					"fqdn": "started-vm.example.test",
					"state": "started",
					"package": {
						"id": "vmpkg_test",
						"permalink": "rock-3"
					},
					"ip_addresses": [],
					"tag_names": []
				}
			}`)
		case r.Method == http.MethodGet &&
			r.URL.Path ==
				"/virtual_machines/virtual_machine/network_interfaces":
			writeTestJSON(w, http.StatusOK, `{
				"pagination": {"total_pages": 1},
				"virtual_machine_network_interfaces": []
			}`)
		default:
			http.NotFound(w, r)
		}
	})
	resource := &VirtualMachineResource{M: &Meta{Core: client, testMode: true}}
	emptyStrings := types.SetValueMust(types.StringType, nil)
	stateModel := VirtualMachineResourceModel{
		ID:                types.StringValue("vm_started"),
		Name:              types.StringValue("Started VM"),
		Hostname:          types.StringValue("started-vm"),
		Description:       types.StringValue("Old description"),
		FQDN:              types.StringValue("started-vm.example.test"),
		State:             types.StringValue("started"),
		PoweredOn:         types.BoolValue(true),
		Package:           types.StringValue("rock-3"),
		DiskTemplate:      types.StringValue("ubuntu-18-04"),
		SystemDisk:        knownTestSystemDisk(t, "disk_vm_started"),
		IPAddressIDs:      emptyStrings,
		IPAddresses:       emptyStrings,
		VirtualNetworkIDs: emptyStrings,
		Tags:              emptyStrings,
	}
	planModel := stateModel
	planModel.Description = types.StringValue("New description")
	planModel.State = types.StringUnknown()

	state := virtualMachineTestState(t, resource, stateModel)
	planState := virtualMachineTestState(t, resource, planModel)
	resp := frameworkresource.UpdateResponse{State: tfsdk.State{
		Schema: state.Schema,
	}}

	resource.Update(context.Background(), frameworkresource.UpdateRequest{
		Config: tfsdk.Config(planState),
		Plan:   tfsdk.Plan(planState),
		State:  state,
	}, &resp)

	require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics.Errors())
	require.Equal(t, 1, patchCalls)
	require.Zero(t, powerCalls)
	var got VirtualMachineResourceModel
	diags := resp.State.Get(context.Background(), &got)
	require.False(t, diags.HasError(), diags.Errors())
	require.True(t, got.PoweredOn.ValueBool())
}

func TestVirtualMachineResourceCreateShutdownFailureKeepsCheckpointedID(
	t *testing.T,
) {
	t.Parallel()

	client := newVirtualMachineTestClient(t, func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		switch {
		case r.Method == http.MethodGet &&
			r.URL.Path == "/ip_addresses/ip_address":
			writeTestJSON(w, http.StatusOK, `{
				"ip_address": {
					"id": "ip_test",
					"network": {"id": "net_test"}
				}
			}`)
		case r.Method == http.MethodPost &&
			r.URL.Path ==
				"/organizations/organization/virtual_machines/build_from_spec":
			writeTestJSON(w, http.StatusCreated, `{
				"annotations": [],
				"build": {"id": "vmbuild_test", "state": "pending"},
				"virtual_machine_build": {
					"id": "vmbuild_test",
					"state": "pending"
				},
				"task": {"id": "task_build", "status": "pending"},
				"hostname": "checkpoint-vm"
			}`)
		case r.Method == http.MethodGet &&
			r.URL.Path == "/virtual_machines/builds/virtual_machine_build":
			writeTestJSON(w, http.StatusOK, `{
				"virtual_machine_build": {
					"id": "vmbuild_test",
					"state": "complete",
					"virtual_machine": {
						"id": "vm_checkpoint",
						"state": "started"
					}
				}
			}`)
		case r.Method == http.MethodGet &&
			r.URL.Path == "/virtual_machines/virtual_machine":
			writeVirtualMachinePowerState(w, "vm_checkpoint", core.Started)
		case r.Method == http.MethodPost &&
			r.URL.Path == "/virtual_machines/virtual_machine/shutdown":
			writeTestJSON(w, http.StatusForbidden, `{
				"error": {
					"code": "permission_denied",
					"description": "Cannot shut down"
				}
			}`)
		default:
			http.NotFound(w, r)
		}
	})
	resource := &VirtualMachineResource{M: &Meta{
		Core:             client,
		confDataCenter:   "test-dc",
		confOrganization: "test-org",
		testMode:         true,
	}}
	model := VirtualMachineResourceModel{
		Name:              types.StringValue("Checkpoint VM"),
		Hostname:          types.StringValue("checkpoint-vm"),
		PoweredOn:         types.BoolValue(false),
		Package:           types.StringValue("rock-3"),
		DiskTemplate:      types.StringValue("ubuntu-18-04"),
		IPAddressIDs:      types.SetValueMust(types.StringType, []attr.Value{types.StringValue("ip_test")}),
		VirtualNetworkIDs: types.SetValueMust(types.StringType, nil),
		Tags:              types.SetValueMust(types.StringType, nil),
	}
	plan := virtualMachineTestState(t, resource, model)
	resp := frameworkresource.CreateResponse{State: tfsdk.State{
		Schema: plan.Schema,
	}}

	resource.Create(context.Background(), frameworkresource.CreateRequest{
		Config: tfsdk.Config(plan),
		Plan:   tfsdk.Plan(plan),
	}, &resp)

	require.True(t, resp.Diagnostics.HasError())
	require.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "permission_denied")
	var got VirtualMachineResourceModel
	diags := resp.State.Get(context.Background(), &got)
	require.False(t, diags.HasError(), diags.Errors())
	require.Equal(t, "vm_checkpoint", got.ID.ValueString())
}

func TestVirtualMachineGroupDataSourceNotFoundDiagnostic(t *testing.T) {
	t.Parallel()

	client := newVirtualMachineTestClient(t, func(
		w http.ResponseWriter,
		_ *http.Request,
	) {
		writeTestJSON(w, http.StatusNotFound, `{
			"error": {
				"code": "virtual_machine_group_not_found",
				"description": "No virtual machine group was found"
			}
		}`)
	})
	ds := &VirtualMachineGroupDataSource{M: &Meta{Core: client, testMode: true}}
	schemaResp := &frameworkdatasource.SchemaResponse{}
	ds.Schema(
		context.Background(),
		frameworkdatasource.SchemaRequest{},
		schemaResp,
	)
	configState := tfsdk.State{Schema: schemaResp.Schema}
	diags := configState.Set(
		context.Background(),
		VirtualMachineGroupDataSourceModel{
			ID: types.StringValue("vmgrp_missing"),
		},
	)
	require.False(t, diags.HasError(), diags.Errors())
	resp := frameworkdatasource.ReadResponse{State: tfsdk.State{
		Schema: schemaResp.Schema,
	}}

	ds.Read(context.Background(), frameworkdatasource.ReadRequest{
		Config: tfsdk.Config{Raw: configState.Raw, Schema: schemaResp.Schema},
	}, &resp)

	require.True(t, resp.Diagnostics.HasError())
	require.Equal(
		t,
		"Virtual Machine Group Not Found",
		resp.Diagnostics.Errors()[0].Summary(),
	)
}

func TestVMReadPreservesConfiguredEmptyOptionalStrings(t *testing.T) {
	t.Parallel()

	client := newVirtualMachineTestClient(t, virtualMachineReadTestHandler)
	resource := &VirtualMachineResource{M: &Meta{Core: client, testMode: true}}

	tests := []struct {
		name            string
		description     types.String
		groupID         types.String
		wantDescription types.String
		wantGroupID     types.String
	}{
		{
			name:            "create plan preserves explicit description",
			description:     types.StringValue(""),
			groupID:         types.StringNull(),
			wantDescription: types.StringValue(""),
			wantGroupID:     types.StringNull(),
		},
		{
			name:            "update plan preserves explicit group",
			description:     types.StringNull(),
			groupID:         types.StringValue(""),
			wantDescription: types.StringNull(),
			wantGroupID:     types.StringValue(""),
		},
		{
			name:            "read state preserves both explicit empty values",
			description:     types.StringValue(""),
			groupID:         types.StringValue(""),
			wantDescription: types.StringValue(""),
			wantGroupID:     types.StringValue(""),
		},
		{
			name:            "omitted values remain null",
			description:     types.StringNull(),
			groupID:         types.StringNull(),
			wantDescription: types.StringNull(),
			wantGroupID:     types.StringNull(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			model := VirtualMachineResourceModel{
				ID:          types.StringValue("vm_test"),
				Description: tt.description,
				GroupID:     tt.groupID,
			}
			err := resource.vmRead(context.Background(), &model)

			require.NoError(t, err)
			require.True(t, model.Description.Equal(tt.wantDescription))
			require.True(t, model.GroupID.Equal(tt.wantGroupID))
		})
	}
}

func TestVMReadHandlesNullBootDiskInstallation(t *testing.T) {
	t.Parallel()

	client := newVirtualMachineTestClient(t, virtualMachineReadTestHandler)
	r := &VirtualMachineResource{M: &Meta{Core: client, testMode: true}}
	model := VirtualMachineResourceModel{
		ID:           types.StringValue("vm_test"),
		DiskTemplate: types.StringValue("ubuntu-18-04"),
	}

	require.NoError(t, r.vmRead(context.Background(), &model))
	require.Equal(t, "ubuntu-18-04", model.DiskTemplate.ValueString())
}

func TestVMReadClearsSystemDiskWhenNoRelationshipsRemain(t *testing.T) {
	t.Parallel()

	client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/virtual_machines/virtual_machine/disks" {
			writeTestJSON(w, http.StatusOK, `{"pagination":{"total_pages":1},"disks":[]}`)
			return
		}
		virtualMachineReadTestHandler(w, req)
	})
	r := &VirtualMachineResource{M: &Meta{Core: client, testMode: true}}
	model := VirtualMachineResourceModel{
		ID: types.StringValue("vm_test"), SystemDisk: knownTestSystemDisk(t, "disk_boot"),
	}

	require.NoError(t, r.vmRead(context.Background(), &model))
	require.True(t, model.SystemDisk.IsNull())
}

func TestVirtualMachineReadBootDiskNotFoundDoesNotRemoveVM(t *testing.T) {
	t.Parallel()

	client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/disks/disk" {
			writeTestJSON(w, http.StatusNotFound, `{
				"error":{"code":"disk_not_found","description":"No disk was found"}
			}`)
			return
		}
		virtualMachineReadTestHandler(w, req)
	})
	r := &VirtualMachineResource{M: &Meta{Core: client, testMode: true}}
	state := virtualMachineTestState(t, r, VirtualMachineResourceModel{
		ID: types.StringValue("vm_test"), SystemDisk: knownTestSystemDisk(t, "disk_boot"),
	})
	resp := frameworkresource.ReadResponse{State: state}

	r.Read(context.Background(), frameworkresource.ReadRequest{State: state}, &resp)

	require.True(t, resp.Diagnostics.HasError())
	require.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "fetching boot disk disk_boot")
	require.False(t, resp.State.Raw.IsNull(), "a missing boot subresource must not remove the VM")
}

func TestVMReadBootDiscoveryFailureRequiresPriorIdentity(t *testing.T) {
	t.Parallel()
	client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/virtual_machines/virtual_machine/disks" {
			writeTestJSON(w, http.StatusInternalServerError, `{"error":{"code":"temporary_failure"}}`)
			return
		}
		virtualMachineReadTestHandler(w, req)
	})
	r := &VirtualMachineResource{M: &Meta{Core: client, testMode: true}}

	t.Run("missing prior boot identity returns discovery error", func(t *testing.T) {
		t.Parallel()
		model := VirtualMachineResourceModel{ID: types.StringValue("vm_test")}
		err := r.vmRead(context.Background(), &model)
		require.ErrorContains(t, err, "discovering boot disk")
	})

	t.Run("known prior boot identity does not suppress discovery error", func(t *testing.T) {
		t.Parallel()
		prior := VirtualMachineSystemDiskModel{
			ID: types.StringValue("disk_boot"), Name: types.StringValue("System Disk"),
			SizeInGB: types.Int64Value(20), ResizeMethod: types.StringValue("offline"),
			WWN: types.StringNull(), State: types.StringValue("built"),
		}
		value, diags := virtualMachineSystemDiskValue(context.Background(), prior)
		require.False(t, diags.HasError(), diags.Errors())
		model := VirtualMachineResourceModel{ID: types.StringValue("vm_test"), SystemDisk: value}
		err := r.vmRead(context.Background(), &model)
		require.ErrorContains(t, err, "discovering boot disk")
		got, gotDiags := decodeVirtualMachineSystemDisk(context.Background(), model.SystemDisk)
		require.False(t, gotDiags.HasError(), gotDiags.Errors())
		require.Equal(t, "disk_boot", got.ID.ValueString())
	})
}

func TestVirtualMachineResourceUpdateRejectsStaleSystemDiskBeforeMutation(t *testing.T) {
	t.Parallel()
	mutations := 0
	client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/virtual_machines/virtual_machine/disks":
			writeTestJSON(w, http.StatusOK, `{
				"pagination":{"total_pages":1},
				"disks":[{"disk":{"id":"disk_current"},"boot":true,"attach_on_boot":true,"state":"attached"}]
			}`)
		case req.Method == http.MethodGet && req.URL.Path == "/disks/disk":
			writeTestJSON(w, http.StatusOK, `{
				"disk":{"id":"disk_current","name":"Current","size_in_gb":20,"state":"built"}
			}`)
		case req.Method == http.MethodPatch || req.Method == http.MethodPut:
			mutations++
			http.Error(w, "unexpected mutation", http.StatusInternalServerError)
		default:
			http.NotFound(w, req)
		}
	})
	r := &VirtualMachineResource{M: &Meta{Core: client, testMode: true}}
	stateModel := VirtualMachineResourceModel{
		ID:         types.StringValue("vm_test"),
		SystemDisk: knownTestSystemDisk(t, "disk_stale"),
	}
	planModel := stateModel
	prior, diags := decodeVirtualMachineSystemDisk(context.Background(), stateModel.SystemDisk)
	require.False(t, diags.HasError(), diags.Errors())
	prior.Name = types.StringValue("Renamed")
	planModel.SystemDisk, diags = virtualMachineSystemDiskValue(context.Background(), prior)
	require.False(t, diags.HasError(), diags.Errors())
	state := virtualMachineTestState(t, r, stateModel)
	plan := virtualMachineTestState(t, r, planModel)
	resp := frameworkresource.UpdateResponse{State: tfsdk.State{Schema: state.Schema}}

	r.Update(context.Background(), frameworkresource.UpdateRequest{
		Config: tfsdk.Config(plan), Plan: tfsdk.Plan(plan), State: state,
	}, &resp)

	require.True(t, resp.Diagnostics.HasError())
	require.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "no longer matches")
	require.Zero(t, mutations)
}

func TestVirtualMachineResourceUpdateRejectsTransitionalSystemDiskResizeBeforeMutation(t *testing.T) {
	t.Parallel()
	for _, vmState := range []string{"starting", "stopping", "failed", "future_state"} {
		t.Run(vmState, func(t *testing.T) {
			t.Parallel()
			resizeCalls := 0
			client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, req *http.Request) {
				switch {
				case req.Method == http.MethodGet && req.URL.Path == "/virtual_machines/virtual_machine/disks":
					writeTestJSON(w, http.StatusOK, `{
						"pagination":{"total_pages":1},
						"disks":[{"disk":{"id":"disk_boot"},"boot":true,"attach_on_boot":true,"state":"attached"}]
					}`)
				case req.Method == http.MethodGet && req.URL.Path == "/disks/disk":
					writeTestJSON(w, http.StatusOK, `{
						"disk":{"id":"disk_boot","name":"System Disk","size_in_gb":20,"state":"built"}
					}`)
				case req.Method == http.MethodGet && req.URL.Path == "/virtual_machines/virtual_machine":
					writeTestJSON(w, http.StatusOK, fmt.Sprintf(`{
						"virtual_machine":{"id":"vm_test","state":%q}
					}`, vmState))
				case req.Method == http.MethodPut && req.URL.Path == "/disks/disk/resize":
					resizeCalls++
					http.Error(w, "unexpected resize", http.StatusInternalServerError)
				default:
					http.NotFound(w, req)
				}
			})
			r := &VirtualMachineResource{M: &Meta{Core: client, testMode: true}}
			stateModel := VirtualMachineResourceModel{
				ID:         types.StringValue("vm_test"),
				SystemDisk: knownTestSystemDisk(t, "disk_boot"),
			}
			planModel := stateModel
			planned, diags := decodeVirtualMachineSystemDisk(context.Background(), planModel.SystemDisk)
			require.False(t, diags.HasError(), diags.Errors())
			planned.SizeInGB = types.Int64Value(30)
			planModel.SystemDisk, diags = virtualMachineSystemDiskValue(context.Background(), planned)
			require.False(t, diags.HasError(), diags.Errors())
			state := virtualMachineTestState(t, r, stateModel)
			plan := virtualMachineTestState(t, r, planModel)
			resp := frameworkresource.UpdateResponse{State: tfsdk.State{Schema: state.Schema}}

			r.Update(context.Background(), frameworkresource.UpdateRequest{
				Config: tfsdk.Config(plan), Plan: tfsdk.Plan(plan), State: state,
			}, &resp)

			require.True(t, resp.Diagnostics.HasError())
			require.Contains(t, resp.Diagnostics.Errors()[0].Detail(), vmState)
			require.Zero(t, resizeCalls)
		})
	}
}

func TestVirtualMachineDataSourceSDKv2CompatibilityAttributes(t *testing.T) {
	t.Parallel()

	ds := &VirtualMachineDataSource{}
	schemaResp := &frameworkdatasource.SchemaResponse{}
	ds.Schema(
		context.Background(),
		frameworkdatasource.SchemaRequest{},
		schemaResp,
	)

	diskTemplateAttr := schemaResp.Schema.Attributes["disk_template"]
	diskTemplate, ok := diskTemplateAttr.(datasourceschema.StringAttribute)
	require.True(t, ok)
	require.True(t, diskTemplate.Computed)

	idAttrValue := schemaResp.Schema.Attributes["id"]
	idAttr, ok := idAttrValue.(datasourceschema.StringAttribute)
	require.True(t, ok)

	selectorsHaveError := func(id, fqdn types.String) bool {
		t.Helper()

		configState := tfsdk.State{Schema: schemaResp.Schema}
		diags := configState.Set(
			context.Background(),
			VirtualMachineDataSourceModel{
				ID:                  id,
				FQDN:                fqdn,
				DiskTemplateOptions: types.MapNull(types.StringType),
				IPAddressIDs:        types.SetNull(types.StringType),
				IPAddresses:         types.SetNull(types.StringType),
				VirtualNetworkIDs:   types.SetNull(types.StringType),
				NetworkInterfaces: types.ListNull(types.ObjectType{
					AttrTypes: vmNetworkInterfaceAttrTypes,
				}),
				Tags: types.SetNull(types.StringType),
			},
		)
		require.False(t, diags.HasError(), diags.Errors())

		validationResp := &frameworkvalidator.StringResponse{}
		validationReq := frameworkvalidator.StringRequest{
			Config:         tfsdk.Config(configState),
			ConfigValue:    id,
			Path:           path.Root("id"),
			PathExpression: path.MatchRoot("id"),
		}
		for _, validator := range idAttr.Validators {
			validator.ValidateString(
				context.Background(), validationReq, validationResp,
			)
		}

		return validationResp.Diagnostics.HasError()
	}
	require.True(
		t,
		selectorsHaveError(types.StringNull(), types.StringNull()),
		"id and fqdn cannot both be omitted",
	)
	require.False(
		t,
		selectorsHaveError(
			types.StringNull(), types.StringValue("vm.example.test"),
		),
		"fqdn should satisfy selector validation",
	)
	require.False(
		t,
		selectorsHaveError(types.StringValue("vm_test"), types.StringNull()),
		"id should satisfy selector validation",
	)

	optionsAttr := schemaResp.Schema.Attributes["disk_template_options"]
	options, ok := optionsAttr.(datasourceschema.MapAttribute)
	require.True(t, ok)
	require.True(t, options.Computed)
	require.Equal(t, types.StringType, options.ElementType)

	client := newVirtualMachineTestClient(t, virtualMachineReadTestHandler)
	ds.M = &Meta{Core: client, testMode: true}
	configState := tfsdk.State{Schema: schemaResp.Schema}
	configModel := VirtualMachineDataSourceModel{
		ID:                  types.StringValue("vm_test"),
		FQDN:                types.StringValue("ignored.example.test"),
		DiskTemplate:        types.StringNull(),
		DiskTemplateOptions: types.MapNull(types.StringType),
		IPAddressIDs:        types.SetNull(types.StringType),
		IPAddresses:         types.SetNull(types.StringType),
		VirtualNetworkIDs:   types.SetNull(types.StringType),
		NetworkInterfaces: types.ListNull(types.ObjectType{
			AttrTypes: vmNetworkInterfaceAttrTypes,
		}),
		Tags: types.SetNull(types.StringType),
	}
	diags := configState.Set(context.Background(), configModel)
	require.False(t, diags.HasError(), diags.Errors())
	req := frameworkdatasource.ReadRequest{Config: tfsdk.Config{
		Raw:    configState.Raw,
		Schema: schemaResp.Schema,
	}}
	resp := frameworkdatasource.ReadResponse{State: tfsdk.State{
		Schema: schemaResp.Schema,
	}}

	ds.Read(context.Background(), req, &resp)

	require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics.Errors())
	var got VirtualMachineDataSourceModel
	diags = resp.State.Get(context.Background(), &got)
	require.False(t, diags.HasError(), diags.Errors())
	require.True(t, got.DiskTemplate.IsNull())
	require.True(t, got.DiskTemplateOptions.IsNull())
	require.True(t, got.NetworkSpeedProfile.IsNull())
}

func TestVirtualMachineResourceDiskTemplateRejectsEmpty(t *testing.T) {
	t.Parallel()

	resource := &VirtualMachineResource{}
	schemaResp := &frameworkresource.SchemaResponse{}
	resource.Schema(
		context.Background(),
		frameworkresource.SchemaRequest{},
		schemaResp,
	)

	diskTemplateAttr := schemaResp.Schema.Attributes["disk_template"]
	diskTemplate, ok := diskTemplateAttr.(resourceschema.StringAttribute)
	require.True(t, ok)
	require.NotEmpty(t, diskTemplate.Validators)

	validationResp := &frameworkvalidator.StringResponse{}
	validationReq := frameworkvalidator.StringRequest{
		ConfigValue: types.StringValue(""),
	}
	for _, validator := range diskTemplate.Validators {
		validator.ValidateString(
			context.Background(),
			validationReq,
			validationResp,
		)
	}
	require.True(t, validationResp.Diagnostics.HasError())
}

func TestVirtualMachineResourceTimeoutsUseBlockSyntax(t *testing.T) {
	t.Parallel()

	resource := &VirtualMachineResource{}
	schemaResp := &frameworkresource.SchemaResponse{}
	resource.Schema(
		context.Background(),
		frameworkresource.SchemaRequest{},
		schemaResp,
	)

	timeoutsBlockValue := schemaResp.Schema.Blocks["timeouts"]
	timeoutsBlock, ok := timeoutsBlockValue.(resourceschema.SingleNestedBlock)
	require.True(t, ok)
	require.Len(t, timeoutsBlock.Attributes, 3)
	require.Contains(t, timeoutsBlock.Attributes, "create")
	require.Contains(t, timeoutsBlock.Attributes, "update")
	require.Contains(t, timeoutsBlock.Attributes, "delete")
}

func TestVirtualMachineResourcePoweredOnSchemaIsNullableObservation(t *testing.T) {
	t.Parallel()

	resource := &VirtualMachineResource{}
	schemaResp := &frameworkresource.SchemaResponse{}
	resource.Schema(
		context.Background(),
		frameworkresource.SchemaRequest{},
		schemaResp,
	)

	attribute, ok := schemaResp.Schema.Attributes["powered_on"].(resourceschema.BoolAttribute)
	require.True(t, ok)
	require.True(t, attribute.Optional)
	require.True(t, attribute.Computed)
	require.Empty(t, attribute.PlanModifiers)
}

func TestVirtualMachineResourceLegacyDiskReplacementIsResourceLevel(t *testing.T) {
	t.Parallel()

	resource := &VirtualMachineResource{}
	schemaResp := &frameworkresource.SchemaResponse{}
	resource.Schema(
		context.Background(),
		frameworkresource.SchemaRequest{},
		schemaResp,
	)
	diskBlockValue := schemaResp.Schema.Blocks["disk"]
	diskBlock, ok := diskBlockValue.(resourceschema.ListNestedBlock)
	require.True(t, ok)
	require.Empty(t, diskBlock.PlanModifiers)
	require.NotEmpty(t, diskBlock.DeprecationMessage)
	size := diskBlock.NestedObject.Attributes[virtualMachineDiskSizeAttribute].(resourceschema.Int64Attribute)
	require.Empty(t, size.Validators)
}

//nolint:lll // Compact model construction keeps each classifier case readable.
func TestVirtualMachineModifyPlanLegacyDiskClassification(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		stateDisks  []VirtualMachineDiskModel
		planDisks   []VirtualMachineDiskModel
		wantError   string
		wantReplace bool
	}{
		{
			name:       "unchanged historical sub minimum disk remains valid",
			stateDisks: legacyDiskModels(8), planDisks: legacyDiskModels(8),
		},
		{
			name:       "new sub minimum disk is rejected",
			stateDisks: legacyDiskModels(20), planDisks: legacyDiskModels(20, 8),
			wantError: "at least 10 GB",
		},
		{
			name:       "edited sub minimum disk is rejected",
			stateDisks: legacyDiskModels(20), planDisks: legacyDiskModels(8),
			wantError: "at least 10 GB",
		},
		{
			name:       "partial removal retaining sub minimum disk is rejected",
			stateDisks: legacyDiskModels(8, 20), planDisks: legacyDiskModels(8),
			wantError: "at least 10 GB",
		},
		{
			name:       "valid edited disk requires replacement",
			stateDisks: legacyDiskModels(20), planDisks: legacyDiskModels(21), wantReplace: true,
		},
		{
			name:       "valid added disk requires replacement",
			stateDisks: legacyDiskModels(20), planDisks: legacyDiskModels(20, 10), wantReplace: true,
		},
		{
			name:       "partial removal requires replacement",
			stateDisks: legacyDiskModels(20, 10), planDisks: legacyDiskModels(20), wantReplace: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			r := &VirtualMachineResource{M: &Meta{}}
			state := VirtualMachineResourceModel{ID: types.StringValue("vm_test"), Disk: legacyDiskList(t, test.stateDisks)}
			plan := state
			plan.Disk = legacyDiskList(t, test.planDisks)
			resp := runVirtualMachineModifyPlan(t, r, state, plan, plan)
			if test.wantError != "" {
				require.True(t, resp.Diagnostics.HasError())
				require.Contains(t, resp.Diagnostics.Errors()[0].Detail(), test.wantError)
				return
			}
			require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics.Errors())
			require.Equal(t, test.wantReplace, len(resp.RequiresReplace) > 0)
		})
	}
}

func TestVirtualMachineModifyPlanRejectsSubMinimumLegacyDiskOnCreate(t *testing.T) {
	t.Parallel()
	r := &VirtualMachineResource{}
	model := VirtualMachineResourceModel{Disk: legacyDiskList(t, legacyDiskModels(8))}
	plan := virtualMachineTestState(t, r, model)
	req := frameworkresource.ModifyPlanRequest{
		Config: tfsdk.Config(plan),
		Plan:   tfsdk.Plan(plan),
		State:  tfsdk.State{Schema: plan.Schema},
	}
	resp := frameworkresource.ModifyPlanResponse{Plan: tfsdk.Plan(plan)}

	r.ModifyPlan(context.Background(), req, &resp)

	require.True(t, resp.Diagnostics.HasError())
	require.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "at least 10 GB")
}

//nolint:lll // Inline API fixtures keep each migration observation self-contained.
func TestVirtualMachineModifyPlanFullLegacyRemoval(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		configureSystem bool
		ambiguousBoot   bool
		wantError       string
	}{
		{name: "omitted system disk migrates without replacement"},
		{name: "equivalent system disk migrates without replacement", configureSystem: true},
		{name: "ambiguous boot relationship is rejected", ambiguousBoot: true, wantError: "authoritative boot disk"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, req *http.Request) {
				require.Equal(t, "/virtual_machines/virtual_machine/disks", req.URL.Path)
				bootJSON, dataBootJSON := `"boot":true`, `"boot":false`
				if test.ambiguousBoot {
					bootJSON = `"boot":null`
					dataBootJSON = `"boot":null`
				}
				writeTestJSON(w, http.StatusOK, fmt.Sprintf(`{
					"pagination":{"current_page":1,"total_pages":1,"total":2,"per_page":30,"large_set":false},
					"disks":[
						{"disk":{"id":"disk_boot","name":"System Disk","size_in_gb":20},%s,"attach_on_boot":true,"state":"attached"},
						{"disk":{"id":"disk_data","name":"Data","size_in_gb":10},%s,"attach_on_boot":true,"state":"attached"}
					]
				}`, bootJSON, dataBootJSON))
			})
			r := &VirtualMachineResource{M: &Meta{Core: client, testMode: true}}
			system := VirtualMachineSystemDiskModel{
				ID: types.StringValue("disk_boot"), Name: types.StringValue("System Disk"),
				SizeInGB: types.Int64Value(20), ResizeMethod: types.StringValue("offline"),
				WWN: types.StringNull(), State: types.StringNull(),
			}
			if test.ambiguousBoot {
				system.ID = types.StringValue("")
			}
			systemValue, diags := virtualMachineSystemDiskValue(context.Background(), system)
			require.False(t, diags.HasError(), diags.Errors())
			state := VirtualMachineResourceModel{
				ID: types.StringValue("vm_test"), Disk: legacyDiskList(t, legacyDiskModels(20, 10)),
				SystemDisk: systemValue,
			}
			plan := state
			plan.Disk = legacyDiskList(t, nil)
			config := plan
			if !test.configureSystem {
				config.SystemDisk = types.ObjectNull(virtualMachineSystemDiskAttrTypes)
			}
			resp := runVirtualMachineModifyPlan(t, r, state, plan, config)
			if test.wantError != "" {
				require.True(t, resp.Diagnostics.HasError())
				require.Contains(t, resp.Diagnostics.Errors()[0].Detail(), test.wantError)
				return
			}
			require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics.Errors())
			require.Empty(t, resp.RequiresReplace)
			require.NotEmpty(t, resp.Diagnostics.Warnings())
		})
	}
}

func legacyDiskModels(sizes ...int64) []VirtualMachineDiskModel {
	disks := make([]VirtualMachineDiskModel, 0, len(sizes))
	for i, size := range sizes {
		disks = append(disks, VirtualMachineDiskModel{
			Name: types.StringValue(fmt.Sprintf("Disk %d", i+1)),
			Size: types.Int64Value(size),
		})
	}
	return disks
}

func knownTestSystemDisk(t *testing.T, id string) types.Object {
	t.Helper()
	value, diags := virtualMachineSystemDiskValue(context.Background(), VirtualMachineSystemDiskModel{
		ID: types.StringValue(id), Name: types.StringValue("System Disk"),
		SizeInGB: types.Int64Value(20), ResizeMethod: types.StringValue("offline"),
		WWN: types.StringNull(), State: types.StringValue("built"),
	})
	require.False(t, diags.HasError(), diags.Errors())
	return value
}

func legacyDiskList(t *testing.T, disks []VirtualMachineDiskModel) types.List {
	t.Helper()
	value, diags := types.ListValueFrom(context.Background(), types.ObjectType{
		AttrTypes: map[string]attr.Type{"name": types.StringType, "size": types.Int64Type},
	}, disks)
	require.False(t, diags.HasError(), diags.Errors())
	return value
}

func runVirtualMachineModifyPlan(
	t *testing.T,
	r *VirtualMachineResource,
	stateModel, planModel, configModel VirtualMachineResourceModel,
) frameworkresource.ModifyPlanResponse {
	t.Helper()
	state := virtualMachineTestState(t, r, stateModel)
	plan := virtualMachineTestState(t, r, planModel)
	config := virtualMachineTestState(t, r, configModel)
	req := frameworkresource.ModifyPlanRequest{
		Config: tfsdk.Config(config),
		Plan:   tfsdk.Plan(plan),
		State:  state,
	}
	resp := frameworkresource.ModifyPlanResponse{Plan: tfsdk.Plan(plan)}
	r.ModifyPlan(context.Background(), req, &resp)
	return resp
}

func TestVirtualMachineResourceSystemDiskSchema(t *testing.T) {
	t.Parallel()
	resource := &VirtualMachineResource{}
	resp := &frameworkresource.SchemaResponse{}
	resource.Schema(context.Background(), frameworkresource.SchemaRequest{}, resp)
	require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics.Errors())

	_, hasDiskIDs := resp.Schema.Attributes["disk_ids"]
	require.False(t, hasDiskIDs)
	systemDisk, ok := resp.Schema.Attributes["system_disk"].(resourceschema.SingleNestedAttribute)
	require.True(t, ok)
	require.True(t, systemDisk.Optional)
	require.True(t, systemDisk.Computed)
	require.NotEmpty(t, systemDisk.PlanModifiers)
	require.NotEmpty(t, systemDisk.Attributes["id"].(resourceschema.StringAttribute).MarkdownDescription)
	name := systemDisk.Attributes["name"].(resourceschema.StringAttribute)
	require.NotEmpty(t, name.MarkdownDescription)
	require.NotEmpty(t, name.Validators)
	size := systemDisk.Attributes["size_in_gb"].(resourceschema.Int64Attribute)
	require.NotEmpty(t, size.Validators)
	require.NotEmpty(t, size.MarkdownDescription)
	resizeMethod := systemDisk.Attributes["resize_method"].(resourceschema.StringAttribute)
	require.True(t, resizeMethod.Optional)
	require.True(t, resizeMethod.Computed)
	require.NotNil(t, resizeMethod.Default)
	require.Contains(t, resizeMethod.MarkdownDescription, "`online`")
	require.Contains(t, resizeMethod.MarkdownDescription, "`offline`")
	require.NotEmpty(t, systemDisk.Attributes["wwn"].(resourceschema.StringAttribute).MarkdownDescription)
	require.NotEmpty(t, systemDisk.Attributes[stateAttributeName].(resourceschema.StringAttribute).MarkdownDescription)
}

type virtualMachinePrivateState map[string][]byte

func (p virtualMachinePrivateState) GetKey(_ context.Context, key string) ([]byte, diag.Diagnostics) {
	return p[key], nil
}

func TestValidateLegacyDiskDeleteOwnershipUsesExactIdentities(t *testing.T) {
	t.Parallel()

	attachment := func(id string, boot bool) core.GetVirtualMachineDisks200ResponseDisks {
		return core.GetVirtualMachineDisks200ResponseDisks{
			Disk: &core.GetVirtualMachineDisksPartDisk{Id: ptr(id)}, Boot: ptr(boot),
		}
	}
	owned, err := json.Marshal([]string{"disk_boot", "disk_legacy"})
	require.NoError(t, err)
	private := virtualMachinePrivateState{virtualMachineLegacyDiskIDsPrivateKey: owned}
	systemDisk := knownTestSystemDisk(t, "disk_boot")

	require.NoError(t, validateLegacyDiskDeleteOwnership(
		context.Background(), private,
		[]core.GetVirtualMachineDisks200ResponseDisks{attachment("disk_boot", true), attachment("disk_legacy", false)},
		systemDisk,
	))
	err = validateLegacyDiskDeleteOwnership(
		context.Background(), private,
		[]core.GetVirtualMachineDisks200ResponseDisks{attachment("disk_boot", true), attachment("disk_foreign", false)},
		systemDisk,
	)
	require.ErrorContains(t, err, "disk_foreign")
	err = validateLegacyDiskDeleteOwnership(
		context.Background(), nil,
		[]core.GetVirtualMachineDisks200ResponseDisks{attachment("disk_boot", true), attachment("disk_legacy", false)},
		systemDisk,
	)
	require.ErrorContains(t, err, "cannot prove ownership")
	require.NoError(t, validateLegacyDiskDeleteOwnership(
		context.Background(), nil,
		[]core.GetVirtualMachineDisks200ResponseDisks{attachment("disk_boot", true)},
		systemDisk,
	))
}

func TestVirtualMachineModifyPlanAdoptsImportedTemplateFieldsOnce(t *testing.T) {
	t.Parallel()
	resource := &VirtualMachineResource{M: &Meta{}}
	stateModel := VirtualMachineResourceModel{
		ID:           types.StringValue("vm_imported"),
		Package:      types.StringValue("rock-3"),
		DiskTemplate: types.StringValue("templates/ubuntu-18-04"),
		DiskTemplateOptions: types.MapNull(
			types.StringType,
		),
	}
	planModel := stateModel
	planModel.DiskTemplate = types.StringValue("ubuntu-18-04")
	planModel.DiskTemplateOptions = types.MapValueMust(
		types.StringType,
		map[string]attr.Value{"install_agent": types.StringValue("true")},
	)

	state := virtualMachineTestState(t, resource, stateModel)
	plan := virtualMachineTestState(t, resource, planModel)
	req := frameworkresource.ModifyPlanRequest{
		Config: tfsdk.Config(plan),
		Plan:   tfsdk.Plan(plan),
		State:  state,
	}
	resp := frameworkresource.ModifyPlanResponse{Plan: tfsdk.Plan(plan)}
	initializeResourcePrivateState(t, &req, &resp)
	require.False(t, req.Private.SetKey(
		context.Background(), virtualMachineImportDiskTemplatePrivateKey, []byte("true"),
	).HasError())
	require.False(t, req.Private.SetKey(
		context.Background(), virtualMachineImportTemplateOptionsPrivateKey, []byte("true"),
	).HasError())

	resource.ModifyPlan(context.Background(), req, &resp)

	require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics.Errors())
	require.Empty(t, resp.RequiresReplace)
	for _, key := range []string{
		virtualMachineImportDiskTemplatePrivateKey,
		virtualMachineImportTemplateOptionsPrivateKey,
	} {
		value, diags := resp.Private.GetKey(context.Background(), key)
		require.False(t, diags.HasError(), diags.Errors())
		require.Empty(t, value)
	}
}

func TestVirtualMachineModifyPlanGuidesImportedLegacyDiskMigrationWithoutReplacement(t *testing.T) {
	t.Parallel()
	r := &VirtualMachineResource{M: &Meta{}}
	stateModel := VirtualMachineResourceModel{
		ID:                  types.StringValue("vm_imported"),
		DiskTemplate:        types.StringValue("templates/ubuntu-18-04"),
		DiskTemplateOptions: types.MapNull(types.StringType),
		SystemDisk:          knownTestSystemDisk(t, "disk_boot"),
	}
	planModel := stateModel
	planModel.Disk = legacyDiskList(t, legacyDiskModels(20, 10))
	planModel.DiskTemplate = types.StringValue("ubuntu-18-04")
	planModel.DiskTemplateOptions = types.MapValueMust(
		types.StringType,
		map[string]attr.Value{"install_agent": types.StringValue("true")},
	)
	configModel := planModel
	configModel.SystemDisk = types.ObjectNull(virtualMachineSystemDiskAttrTypes)
	state := virtualMachineTestState(t, r, stateModel)
	plan := virtualMachineTestState(t, r, planModel)
	config := virtualMachineTestState(t, r, configModel)
	req := frameworkresource.ModifyPlanRequest{
		Config: tfsdk.Config(config), Plan: tfsdk.Plan(plan), State: state,
	}
	resp := frameworkresource.ModifyPlanResponse{Plan: tfsdk.Plan(plan)}
	initializeResourcePrivateState(t, &req, &resp)
	for _, key := range []string{
		virtualMachineImportDiskTemplatePrivateKey,
		virtualMachineImportTemplateOptionsPrivateKey,
	} {
		require.False(t, req.Private.SetKey(context.Background(), key, []byte("true")).HasError())
	}

	r.ModifyPlan(context.Background(), req, &resp)

	require.True(t, resp.Diagnostics.HasError())
	require.Empty(t, resp.RequiresReplace)
	detail := resp.Diagnostics.Errors()[0].Detail()
	require.Contains(t, detail, "system_disk")
	require.Contains(t, detail, "katapult_disk")
	require.Contains(t, detail, "katapult_disk_assignment")
	for _, key := range []string{
		virtualMachineImportDiskTemplatePrivateKey,
		virtualMachineImportTemplateOptionsPrivateKey,
	} {
		value, diags := resp.Private.GetKey(context.Background(), key)
		require.False(t, diags.HasError(), diags.Errors())
		require.NotEmpty(t, value)
	}
}

func TestVirtualMachineImportMarksTemplateFieldsAdoptable(t *testing.T) {
	t.Parallel()
	r := &VirtualMachineResource{}
	state := virtualMachineTestState(t, r, VirtualMachineResourceModel{})
	resp := frameworkresource.ImportStateResponse{State: state}
	initializeResourcePrivateState(t, &resp, &resp)

	r.ImportState(
		context.Background(),
		frameworkresource.ImportStateRequest{ID: "vm_imported"},
		&resp,
	)

	require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics.Errors())
	var imported VirtualMachineResourceModel
	diags := resp.State.Get(context.Background(), &imported)
	require.False(t, diags.HasError(), diags.Errors())
	require.Equal(t, "vm_imported", imported.ID.ValueString())
	for _, key := range []string{
		virtualMachineImportDiskTemplatePrivateKey,
		virtualMachineImportTemplateOptionsPrivateKey,
	} {
		value, privateDiags := resp.Private.GetKey(context.Background(), key)
		require.False(t, privateDiags.HasError(), privateDiags.Errors())
		require.JSONEq(t, "true", string(value))
	}
}

func virtualMachineTestState(
	t *testing.T,
	resource *VirtualMachineResource,
	model VirtualMachineResourceModel,
) tfsdk.State {
	t.Helper()
	if model.DiskTemplateOptions.IsNull() {
		model.DiskTemplateOptions = types.MapNull(types.StringType)
	}
	if model.Disk.IsNull() {
		model.Disk = types.ListNull(types.ObjectType{
			AttrTypes: map[string]attr.Type{
				"name": types.StringType,
				"size": types.Int64Type,
			},
		})
	}
	if model.IPAddressIDs.IsNull() {
		model.IPAddressIDs = types.SetNull(types.StringType)
	}
	if model.IPAddresses.IsNull() {
		model.IPAddresses = types.SetNull(types.StringType)
	}
	if model.SystemDisk.IsNull() {
		model.SystemDisk = types.ObjectNull(virtualMachineSystemDiskAttrTypes)
	}
	if model.VirtualNetworkIDs.IsNull() {
		model.VirtualNetworkIDs = types.SetNull(types.StringType)
	}
	if model.NetworkInterfaces.IsNull() {
		model.NetworkInterfaces = types.ListNull(types.ObjectType{
			AttrTypes: vmNetworkInterfaceAttrTypes,
		})
	}
	if model.Tags.IsNull() {
		model.Tags = types.SetNull(types.StringType)
	}
	if len(model.Timeouts.AttributeTypes(context.Background())) == 0 {
		model.Timeouts = resourcetimeouts.Value{Object: types.ObjectNull(
			map[string]attr.Type{
				"create": types.StringType,
				"update": types.StringType,
				"delete": types.StringType,
			},
		)}
	}

	schemaResp := &frameworkresource.SchemaResponse{}
	resource.Schema(
		context.Background(),
		frameworkresource.SchemaRequest{},
		schemaResp,
	)
	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(context.Background(), model)
	require.False(t, diags.HasError(), diags.Errors())

	return state
}

func virtualMachineGroupTestState(
	t *testing.T,
	resource *VirtualMachineGroupResource,
	model VirtualMachineGroupResourceModel,
) tfsdk.State {
	t.Helper()

	schemaResp := &frameworkresource.SchemaResponse{}
	resource.Schema(
		context.Background(),
		frameworkresource.SchemaRequest{},
		schemaResp,
	)
	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(context.Background(), model)
	require.False(t, diags.HasError(), diags.Errors())

	return state
}

func newVirtualMachineTestClient(
	t *testing.T,
	handler http.HandlerFunc,
) core.ClientWithResponsesInterface {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client, err := core.NewClientWithResponses(server.URL, "test-token")
	require.NoError(t, err)

	return client
}

func virtualMachineReadTestHandler(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/virtual_machines/virtual_machine":
		writeTestJSON(w, http.StatusOK, `{
			"annotations": [],
			"virtual_machine": {
				"id": "vm_test",
				"name": "Test VM",
				"hostname": "test-vm",
				"fqdn": "test-vm.example.test",
				"state": "stopped",
				"ip_addresses": [],
				"tag_names": []
			}
		}`)
	case "/virtual_machines/virtual_machine/network_interfaces":
		writeTestJSON(w, http.StatusOK, `{
			"pagination": {"total_pages": 1},
			"virtual_machine_network_interfaces": []
		}`)
	case "/virtual_machines/virtual_machine/disks":
		writeTestJSON(w, http.StatusOK, `{
			"pagination": {"total_pages": 1},
			"disks": [{
				"disk": {"id": "disk_boot", "name": "System Disk", "size_in_gb": 20, "state": "built"},
				"attach_on_boot": true,
				"boot": true,
				"state": "attached"
			}]
		}`)
	case "/disks/disk":
		writeTestJSON(w, http.StatusOK, `{
			"disk": {"id": "disk_boot", "name": "System Disk", "size_in_gb": 20, "state": "built", "installation": null}
		}`)
	default:
		http.NotFound(w, r)
	}
}

func writeVirtualMachineBootDiskTestResponse(
	w http.ResponseWriter,
	r *http.Request,
	diskID string,
) bool {
	if r.Method != http.MethodGet {
		return false
	}
	switch r.URL.Path {
	case "/virtual_machines/virtual_machine/disks":
		writeTestJSON(w, http.StatusOK, fmt.Sprintf(`{
			"pagination":{"total_pages":1},
			"disks":[{
				"disk":{"id":%q},
				"boot":true,
				"attach_on_boot":true,
				"state":"attached"
			}]
		}`, diskID))
		return true
	case "/disks/disk":
		writeTestJSON(w, http.StatusOK, fmt.Sprintf(`{
			"disk":{
				"id":%q,
				"name":"System Disk",
				"size_in_gb":20,
				"state":"built"
			}
		}`, diskID))
		return true
	default:
		return false
	}
}

func writeObjectInTrashResponse(w http.ResponseWriter) {
	writeTestJSON(w, http.StatusNotAcceptable, `{
		"code": "object_in_trash",
		"description": "The object is already in trash"
	}`)
}

func writeTestJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
