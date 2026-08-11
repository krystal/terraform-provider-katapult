package v6provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	resourcetimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	frameworkdatasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
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
	require.Len(t, idAttr.Validators, 1)
	fqdnAttrValue := schemaResp.Schema.Attributes["fqdn"]
	fqdnAttr, ok := fqdnAttrValue.(datasourceschema.StringAttribute)
	require.True(t, ok)
	require.Empty(t, fqdnAttr.Validators)

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

	timeoutsAttrValue := schemaResp.Schema.Attributes["timeouts"]
	timeoutsAttr, ok := timeoutsAttrValue.(resourceschema.SingleNestedAttribute)
	require.True(t, ok)
	require.True(t, timeoutsAttr.Optional)
	require.Len(t, timeoutsAttr.Attributes, 3)
	require.Contains(t, timeoutsAttr.Attributes, "create")
	require.Contains(t, timeoutsAttr.Attributes, "update")
	require.Contains(t, timeoutsAttr.Attributes, "delete")
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
	default:
		http.NotFound(w, r)
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
