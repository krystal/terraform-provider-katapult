package v6provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	resourcetimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/next/core"
	"github.com/stretchr/testify/require"
)

func init() { //nolint:gochecknoinits
	resource.AddTestSweepers("katapult_disk", &resource.Sweeper{
		Name: "katapult_disk",
		F:    testSweepDisks,
	})
}

func TestDiskResourceResizeSchema(t *testing.T) {
	t.Parallel()
	r := &DiskResource{}
	resp := &frameworkresource.SchemaResponse{}
	r.Schema(context.Background(), frameworkresource.SchemaRequest{}, resp)
	require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics.Errors())

	size := resp.Schema.Attributes["size_in_gb"].(resourceschema.Int64Attribute)
	require.Empty(t, size.PlanModifiers)
	require.NotEmpty(t, size.Validators)
	method := resp.Schema.Attributes["resize_method"].(resourceschema.StringAttribute)
	require.True(t, method.Optional)
	require.True(t, method.Computed)
	require.NotNil(t, method.Default)
	initialFileSystem := resp.Schema.Attributes["initial_file_system"].(resourceschema.StringAttribute)
	require.True(t, initialFileSystem.Optional)
	require.False(t, initialFileSystem.Computed)
	require.NotEmpty(t, initialFileSystem.Validators)
	require.NotEmpty(t, initialFileSystem.PlanModifiers)
	_, ok := resp.Schema.Blocks["timeouts"].(resourceschema.SingleNestedBlock)
	require.True(t, ok)
}

func TestInitialFileSystemReplacementAfterImportAdoption(t *testing.T) {
	t.Parallel()

	modifier := requiresReplaceAfterImportAdoptionModifier{}
	for _, test := range []struct {
		name        string
		state       types.String
		plan        types.String
		imported    bool
		wantReplace bool
	}{
		{
			name:  "imported null adopts configured filesystem",
			state: types.StringNull(), plan: types.StringValue("ext4"), imported: true,
		},
		{
			name:  "managed blank disk requires replacement",
			state: types.StringNull(), plan: types.StringValue("ext4"), wantReplace: true,
		},
		{name: "unchanged managed filesystem", state: types.StringValue("ext4"), plan: types.StringValue("ext4")},
		{
			name:  "managed filesystem change replaces",
			state: types.StringValue("ext4"), plan: types.StringValue("xfs"),
			wantReplace: true,
		},
		{
			name:  "managed filesystem removal replaces",
			state: types.StringValue("ext4"), plan: types.StringNull(),
			wantReplace: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			req := planmodifier.StringRequest{
				State:      tfsdk.State{Raw: tftypes.NewValue(tftypes.String, "state")},
				Plan:       tfsdk.Plan{Raw: tftypes.NewValue(tftypes.String, "plan")},
				StateValue: test.state,
				PlanValue:  test.plan,
			}
			resp := &planmodifier.StringResponse{PlanValue: test.plan}
			initializeResourcePrivateState(t, &req, resp)
			if test.imported {
				diags := req.Private.SetKey(
					context.Background(), diskImportInitialFileSystemPrivateKey, []byte("true"),
				)
				require.False(t, diags.HasError(), diags.Errors())
			}
			modifier.PlanModifyString(context.Background(), req, resp)
			require.Equal(t, test.wantReplace, resp.RequiresReplace)
			if test.imported {
				value, diags := resp.Private.GetKey(
					context.Background(), diskImportInitialFileSystemPrivateKey,
				)
				require.False(t, diags.HasError(), diags.Errors())
				require.Empty(t, value)
			}
		})
	}
}

func TestDiskImportMarksInitialFileSystemAdoptable(t *testing.T) {
	t.Parallel()

	r := &DiskResource{}
	state := diskTestState(t, r, DiskResourceModel{})
	resp := frameworkresource.ImportStateResponse{State: state}
	initializeResourcePrivateState(t, &resp, &resp)

	r.ImportState(
		context.Background(), frameworkresource.ImportStateRequest{ID: "disk_imported"}, &resp,
	)

	require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics.Errors())
	value, diags := resp.Private.GetKey(
		context.Background(), diskImportInitialFileSystemPrivateKey,
	)
	require.False(t, diags.HasError(), diags.Errors())
	require.NotEmpty(t, value)
}

func TestDiskModifyPlanSkipsResizeValidationForMissingDisk(t *testing.T) {
	t.Parallel()

	client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(w, http.StatusNotFound, `{
			"error":{"code":"disk_not_found","description":"No disk was found"}
		}`)
	})
	r := &DiskResource{M: &Meta{Core: client, testMode: true}}
	state := diskTestState(t, r, DiskResourceModel{
		ID: types.StringValue("disk_missing"), Name: types.StringValue("Data"),
		SizeInGB: types.Int64Value(20), ResizeMethod: types.StringValue("offline"),
	})
	plan := diskTestState(t, r, DiskResourceModel{
		ID: types.StringValue("disk_missing"), Name: types.StringValue("Data"),
		SizeInGB: types.Int64Value(30), ResizeMethod: types.StringValue("offline"),
	})
	req := frameworkresource.ModifyPlanRequest{
		Config: tfsdk.Config(plan), Plan: tfsdk.Plan(plan), State: state,
	}
	resp := frameworkresource.ModifyPlanResponse{Plan: tfsdk.Plan(plan)}

	r.ModifyPlan(context.Background(), req, &resp)

	require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics.Errors())
}

func TestDiskCreateIncludesInitialFileSystem(t *testing.T) {
	t.Parallel()

	var createBody core.PostOrganizationDisksJSONRequestBody
	var createBodyErr error
	client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/organizations/organization/disks":
			createBodyErr = json.NewDecoder(req.Body).Decode(&createBody)
			writeTestJSON(w, http.StatusCreated, `{
				"disk":{"id":"disk_test"},
				"task":{"id":"task_build","status":"pending"}
			}`)
		case req.Method == http.MethodGet && req.URL.Path == "/tasks/task":
			writeTestJSON(w, http.StatusOK, `{
				"task":{"id":"task_build","status":"completed"}
			}`)
		case req.Method == http.MethodGet && req.URL.Path == "/disks/disk":
			writeTestJSON(w, http.StatusOK, `{
				"disk":{
					"id":"disk_test",
					"name":"Test Disk",
					"size_in_gb":20,
					"state":"built",
					"virtual_machine_disk":null
				}
			}`)
		default:
			http.NotFound(w, req)
		}
	})
	r := &DiskResource{M: &Meta{
		Core: client, confDataCenter: "test-dc", confOrganization: "test-org", testMode: true,
	}}
	plan := diskTestState(t, r, DiskResourceModel{
		Name:              types.StringValue("Test Disk"),
		SizeInGB:          types.Int64Value(20),
		InitialFileSystem: types.StringValue("ext4"),
	})
	resp := frameworkresource.CreateResponse{State: tfsdk.State{Schema: plan.Schema}}

	r.Create(
		context.Background(), frameworkresource.CreateRequest{Plan: tfsdk.Plan(plan)}, &resp,
	)
	require.False(t, resp.Diagnostics.HasError(), resp.Diagnostics.Errors())
	require.NoError(t, createBodyErr)
	require.NotNil(t, createBody.Properties.InitialFileSystem)
	require.Equal(t, core.Ext4, *createBody.Properties.InitialFileSystem)

	var state DiskResourceModel
	diags := resp.State.Get(context.Background(), &state)
	require.False(t, diags.HasError(), diags.Errors())
	require.Equal(t, "ext4", state.InitialFileSystem.ValueString())
}

func TestDiskResourceCreateRejectsMalformedSuccessResponse(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		want        string
		wantID      string
	}{
		{name: "empty response", contentType: "text/plain", body: "", want: "unexpected empty response creating disk"},
		{name: "missing disk ID", body: `{"disk":{},"task":{"id":"task_test"}}`, want: "missing disk ID"},
		{
			name: "missing task ID", body: `{"disk":{"id":"disk_test"},"task":{}}`,
			want: "missing task ID", wantID: "disk_test",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, req *http.Request) {
				if req.Method != http.MethodPost || req.URL.Path != "/organizations/organization/disks" {
					http.NotFound(w, req)
					return
				}
				if test.contentType != "" {
					w.Header().Set("Content-Type", test.contentType)
					w.WriteHeader(http.StatusCreated)
					_, _ = w.Write([]byte(test.body))
					return
				}
				writeTestJSON(w, http.StatusCreated, test.body)
			})
			r := &DiskResource{M: &Meta{
				Core: client, confDataCenter: "test-dc", confOrganization: "test-org", testMode: true,
			}}
			plan := diskTestState(t, r, DiskResourceModel{
				Name: types.StringValue("Test Disk"), SizeInGB: types.Int64Value(20),
			})
			resp := frameworkresource.CreateResponse{State: tfsdk.State{Schema: plan.Schema}}

			require.NotPanics(t, func() {
				r.Create(context.Background(), frameworkresource.CreateRequest{Plan: tfsdk.Plan(plan)}, &resp)
			})
			require.True(t, resp.Diagnostics.HasError())
			require.Contains(t, resp.Diagnostics.Errors()[0].Detail(), test.want)
			if test.wantID != "" {
				var state DiskResourceModel
				diags := resp.State.Get(context.Background(), &state)
				require.False(t, diags.HasError(), diags.Errors())
				require.Equal(t, test.wantID, state.ID.ValueString())
			}
		})
	}
}

func TestDiskReadClearsNullableRelationships(t *testing.T) {
	t.Parallel()

	client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet || req.URL.Path != "/disks/disk" {
			http.NotFound(w, req)
			return
		}
		writeTestJSON(w, http.StatusOK, `{
			"disk": {
				"id": "disk_test",
				"name": "Test Disk",
				"size_in_gb": 20,
				"bus_type": null,
				"io_profile": null
			}
		}`)
	})
	r := &DiskResource{M: &Meta{Core: client, testMode: true}}
	model := DiskResourceModel{
		BusType: types.StringValue("virtio"), IOProfileID: types.StringValue("iop_stale"),
		StorageSpeed: types.StringValue("nvme"), WWN: types.StringValue("wwn_stale"),
		State: types.StringValue("built"),
	}

	require.NoError(t, r.diskRead(context.Background(), "disk_test", &model))
	require.True(t, model.BusType.IsNull())
	require.True(t, model.IOProfileID.IsNull())
	require.True(t, model.StorageSpeed.IsNull())
	require.True(t, model.WWN.IsNull())
	require.True(t, model.State.IsNull())
}

func TestDiskResizePropagatesFailedTaskWithoutChangingModels(t *testing.T) {
	t.Parallel()

	var resizeRequests atomic.Int32
	client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/disks/disk":
			writeTestJSON(w, http.StatusOK, `{
				"disk":{"id":"disk_test","size_in_gb":20,"virtual_machine_disk":null}
			}`)
		case req.Method == http.MethodPut && req.URL.Path == "/disks/disk/resize":
			resizeRequests.Add(1)
			writeTestJSON(w, http.StatusOK, `{"task":{"id":"task_failed"}}`)
		case req.Method == http.MethodGet && req.URL.Path == "/tasks/task":
			writeTestJSON(w, http.StatusOK, `{
				"task":{"id":"task_failed","status":"failed"}
			}`)
		default:
			http.NotFound(w, req)
		}
	})
	r := &DiskResource{M: &Meta{Core: client, testMode: true}}
	state := &DiskResourceModel{
		SizeInGB: types.Int64Value(20), ResizeMethod: types.StringValue("offline"),
	}
	plan := &DiskResourceModel{
		SizeInGB: types.Int64Value(30), ResizeMethod: types.StringValue("offline"),
	}

	err := r.resizeDisk(
		context.Background(), "disk_test", plan, state, core.Offline, time.Second,
	)
	require.ErrorContains(t, err, "task failed")
	require.Equal(t, int32(1), resizeRequests.Load())
	require.Equal(t, int64(20), state.SizeInGB.ValueInt64())
	require.Equal(t, int64(30), plan.SizeInGB.ValueInt64())
}

func TestDiskIOProfileUpdateUsesDedicatedTaskOperation(t *testing.T) {
	t.Parallel()

	var patchRequests, profileRequests atomic.Int32
	var profileBody core.PutDiskIoProfileJSONRequestBody
	var profileBodyErr error
	client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodPatch:
			patchRequests.Add(1)
			http.Error(w, "unexpected patch", http.StatusInternalServerError)
		case req.Method == http.MethodPut && req.URL.Path == "/disks/disk/io_profile":
			profileRequests.Add(1)
			profileBodyErr = json.NewDecoder(req.Body).Decode(&profileBody)
			writeTestJSON(w, http.StatusOK, `{
				"disk":{"id":"disk_test"},
				"task":{"id":"task_profile","status":"pending"}
			}`)
		case req.Method == http.MethodGet && req.URL.Path == "/tasks/task":
			writeTestJSON(w, http.StatusOK, `{
				"task":{"id":"task_profile","status":"completed"}
			}`)
		default:
			http.NotFound(w, req)
		}
	})
	r := &DiskResource{M: &Meta{Core: client, testMode: true}}
	state := &DiskResourceModel{
		Name: types.StringValue("Data"), BusType: types.StringValue("virtio"),
		IOProfileID: types.StringValue("iop_old"),
	}
	plan := &DiskResourceModel{
		Name: types.StringValue("Data"), BusType: types.StringValue("virtio"),
		IOProfileID: types.StringValue("iop_new"),
	}

	require.NoError(t, r.patchDiskProperties(
		context.Background(), "disk_test", plan, state, time.Second,
	))
	require.Zero(t, patchRequests.Load())
	require.Equal(t, int32(1), profileRequests.Load())
	require.NoError(t, profileBodyErr)
	require.NotNil(t, profileBody.Disk.Id)
	require.Equal(t, "disk_test", *profileBody.Disk.Id)
	require.NotNil(t, profileBody.IoProfile.Id)
	require.Equal(t, "iop_new", *profileBody.IoProfile.Id)
}

func TestDiskDeleteRejectsRemainingAssignmentBeforeMutation(t *testing.T) {
	t.Parallel()

	var deleteRequests atomic.Int32
	client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/disks/disk":
			writeTestJSON(w, http.StatusOK, `{
				"disk":{
					"id":"disk_test",
					"virtual_machine_disk":{
						"boot":false,
						"attach_on_boot":true,
						"state":"attached",
						"virtual_machine":{"id":"vm_test"}
					}
				}
			}`)
		case req.Method == http.MethodDelete:
			deleteRequests.Add(1)
			http.Error(w, "unexpected delete", http.StatusInternalServerError)
		default:
			http.NotFound(w, req)
		}
	})
	r := &DiskResource{M: &Meta{Core: client, testMode: true}}
	state := diskTestState(t, r, DiskResourceModel{
		ID:           types.StringValue("disk_test"),
		Name:         types.StringValue("Data"),
		SizeInGB:     types.Int64Value(20),
		ResizeMethod: types.StringValue("offline"),
	})
	resp := frameworkresource.DeleteResponse{State: state}

	r.Delete(
		context.Background(), frameworkresource.DeleteRequest{State: state}, &resp,
	)
	require.True(t, resp.Diagnostics.HasError())
	require.Contains(
		t,
		resp.Diagnostics.Errors()[0].Detail(),
		"still has a Virtual Machine assignment",
	)
	require.Zero(t, deleteRequests.Load())
}

func TestDiskDeleteRequiresExplicitUnassignedObservation(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		body string
		want string
	}{
		{name: "empty success response", body: "", want: "incomplete API response"},
		{name: "omitted relationship", body: `{"disk":{"id":"disk_test"}}`, want: "virtual_machine_disk was omitted"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var deleteRequests atomic.Int32
			client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, req *http.Request) {
				switch {
				case req.Method == http.MethodGet && req.URL.Path == "/disks/disk":
					if test.body == "" {
						w.Header().Set("Content-Type", "text/plain")
						w.WriteHeader(http.StatusOK)
					} else {
						writeTestJSON(w, http.StatusOK, test.body)
					}
				case req.Method == http.MethodDelete:
					deleteRequests.Add(1)
					http.Error(w, "unexpected delete", http.StatusInternalServerError)
				default:
					http.NotFound(w, req)
				}
			})
			r := &DiskResource{M: &Meta{Core: client, testMode: true}}
			state := diskTestState(t, r, DiskResourceModel{
				ID: types.StringValue("disk_test"), Name: types.StringValue("Data"),
				SizeInGB: types.Int64Value(20), ResizeMethod: types.StringValue("offline"),
			})
			resp := frameworkresource.DeleteResponse{State: state}

			r.Delete(context.Background(), frameworkresource.DeleteRequest{State: state}, &resp)

			require.True(t, resp.Diagnostics.HasError())
			require.Contains(t, resp.Diagnostics.Errors()[0].Detail(), test.want)
			require.Zero(t, deleteRequests.Load())
		})
	}
}

func diskTestState(
	t *testing.T,
	r *DiskResource,
	model DiskResourceModel,
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

func testSweepDisks(_ string) error {
	m := sweepMeta()
	ctx := context.TODO()

	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		res, err := m.Core.GetOrganizationDisksWithResponse(ctx,
			&core.GetOrganizationDisksParams{
				OrganizationSubDomain: &m.confOrganization,
				Page:                  &pageNum,
			})
		if err != nil {
			return err
		}

		totalPages = res.JSON200.Pagination.TotalPages.MustGet()

		for _, disk := range res.JSON200.Disk {
			if disk.Name == nil {
				continue
			}
			if !strings.HasPrefix(*disk.Name, testAccResourceNamePrefix) {
				continue
			}
			if disk.Id == nil {
				continue
			}

			diskID := *disk.Id
			m.Logger.Info("deleting disk", "id", diskID, "name", *disk.Name)

			delRes, delErr := m.Core.DeleteDiskWithResponse(ctx,
				core.DeleteDiskJSONRequestBody{
					Disk: core.DiskLookup{Id: &diskID},
				})
			if delErr != nil {
				return delErr
			}

			if delRes.JSON200 != nil {
				trashObj := delRes.JSON200.TrashObject
				_, _ = m.Core.DeleteTrashObjectWithResponse(ctx,
					core.DeleteTrashObjectJSONRequestBody{
						TrashObject: core.TrashObjectLookup{
							Id: trashObj.Id,
						},
					})
			}
		}
	}

	return nil
}

func TestAccKatapultDisk_basic(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultDiskDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_disk" "test" {
					  name       = "%s"
					  size_in_gb = 20
					}`, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultDiskExists(
						tt, "katapult_disk.test",
					),
					resource.TestCheckResourceAttr(
						"katapult_disk.test",
						"name", name,
					),
					resource.TestCheckResourceAttr(
						"katapult_disk.test",
						"size_in_gb", "20",
					),
					resource.TestCheckResourceAttrSet(
						"katapult_disk.test", "id",
					),
					resource.TestCheckResourceAttrSet(
						"katapult_disk.test", "wwn",
					),
					resource.TestCheckResourceAttrSet(
						"katapult_disk.test", "state",
					),
					resource.TestCheckResourceAttrSet(
						"katapult_disk.test",
						"storage_speed",
					),
				),
			},
			{
				ResourceName:      "katapult_disk.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultDisk_storage_speed_nvme(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultDiskDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_disk" "test" {
					  name          = "%s"
					  size_in_gb    = 20
					  storage_speed = "nvme"
					}`, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"katapult_disk.test",
						"storage_speed", "nvme",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_disk" "test" {
					  name          = "%s"
					  size_in_gb    = 20
					  storage_speed = "ssd"
					}`, name,
				),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPreRefresh: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(
							"katapult_disk.test", plancheck.ResourceActionReplace,
						),
					},
				},
			},
		},
	})
}

func TestAccKatapultDisk_update(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()
	nameUpdated := name + "-updated"
	var diskID string

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultDiskDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					data "katapult_disk_io_profile" "selected" {
					  permalink = "100k"
					}

					resource "katapult_disk" "test" {
					  name          = "%s"
					  size_in_gb    = 20
					  bus_type      = "virtio"
					  io_profile_id = data.katapult_disk_io_profile.selected.id
					}`, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultDiskExists(
						tt, "katapult_disk.test",
					),
					resource.TestCheckResourceAttr(
						"katapult_disk.test", "name", name,
					),
					resource.TestCheckResourceAttr(
						"katapult_disk.test", "bus_type", "virtio",
					),
					resource.TestCheckResourceAttrPair(
						"katapult_disk.test", "io_profile_id",
						"data.katapult_disk_io_profile.selected", "id",
					),
					captureResourceAttr(
						"katapult_disk.test", "id", &diskID,
					),
				),
			},
			{
				Config: undent.Stringf(`
					data "katapult_disk_io_profile" "selected" {
					  permalink = "unrestricted"
					}

					resource "katapult_disk" "test" {
					  name          = "%s"
					  size_in_gb    = 20
					  bus_type      = "scsi"
					  io_profile_id = data.katapult_disk_io_profile.selected.id
					}`, nameUpdated,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"katapult_disk.test",
						"name", nameUpdated,
					),
					resource.TestCheckResourceAttr(
						"katapult_disk.test", "bus_type", "scsi",
					),
					resource.TestCheckResourceAttrPair(
						"katapult_disk.test", "io_profile_id",
						"data.katapult_disk_io_profile.selected", "id",
					),
					resource.TestCheckResourceAttrPtr(
						"katapult_disk.test", "id", &diskID,
					),
					testAccCheckKatapultDiskExists(tt, "katapult_disk.test"),
				),
			},
		},
	})
}

func TestAccKatapultDisk_resize(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()
	var diskID string

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultDiskDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_disk" "test" {
					  name       = "%s"
					  size_in_gb = 20
					}`, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"katapult_disk.test", "size_in_gb", "20",
					),
					captureResourceAttr("katapult_disk.test", "id", &diskID),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_disk" "test" {
					  name          = "%s"
					  size_in_gb    = 30
					  resize_method = "offline"
					}`, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"katapult_disk.test", "size_in_gb", "30",
					),
					resource.TestCheckResourceAttrPtr(
						"katapult_disk.test", "id", &diskID,
					),
				),
			},
		},
	})
}

func TestAccKatapultDisk_shrink(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()
	var diskID string
	config := func(size int, fileSystem string) string {
		return undent.Stringf(`
			resource "katapult_disk" "test" {
			  name                = "%s"
			  size_in_gb          = %d
			  initial_file_system = "%s"
			  resize_method       = "offline"

			  timeouts {
			    update = "15m"
			  }
			}`, name, size, fileSystem)
	}

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultDiskDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: config(20, "ext4"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"katapult_disk.test", "size_in_gb", "20",
					),
					resource.TestCheckResourceAttr(
						"katapult_disk.test", "initial_file_system", "ext4",
					),
					captureResourceAttr("katapult_disk.test", "id", &diskID),
				),
			},
			{
				Config: config(10, "ext4"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"katapult_disk.test", "size_in_gb", "10",
					),
					resource.TestCheckResourceAttrPtr(
						"katapult_disk.test", "id", &diskID,
					),
				),
			},
			{
				Config:             config(10, "xfs"),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPreRefresh: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(
							"katapult_disk.test", plancheck.ResourceActionReplace,
						),
					},
				},
			},
		},
	})
}

//
// Helper check functions
//

func testAccCheckKatapultDiskExists(
	tt *testTools,
	res string,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("resource not found: %s", res)
		}

		diskRes, err := tt.Meta.Core.GetDiskWithResponse(tt.Ctx,
			&core.GetDiskParams{DiskId: &rs.Primary.ID})
		if err != nil {
			return err
		}
		if diskRes.JSON200 == nil {
			return fmt.Errorf(
				"katapult_disk %s not found", rs.Primary.ID,
			)
		}

		return nil
	}
}

func testAccCheckKatapultDiskDestroy(
	tt *testTools,
) resource.TestCheckFunc {
	m := tt.Meta

	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "katapult_disk" {
				continue
			}

			diskRes, err := m.Core.GetDiskWithResponse(tt.Ctx,
				&core.GetDiskParams{DiskId: &rs.Primary.ID})
			if err == nil && diskRes != nil && diskRes.JSON200 != nil {
				return fmt.Errorf(
					"katapult_disk %s was not destroyed",
					rs.Primary.ID,
				)
			}
			if err != nil {
				if !errors.Is(err, core.ErrNotFound) &&
					(diskRes == nil || diskRes.JSON404 == nil) {
					return err
				}
			}

			trashRes, err := m.Core.GetTrashObjectWithResponse(tt.Ctx,
				&core.GetTrashObjectParams{
					TrashObjectObjectId: &rs.Primary.ID,
				})
			if err == nil && trashRes != nil && trashRes.JSON200 != nil {
				return fmt.Errorf(
					"katapult_disk %s was deleted "+
						"but not purged from trash",
					rs.Primary.ID,
				)
			}
			if err != nil && !errors.Is(err, core.ErrNotFound) &&
				(trashRes == nil || trashRes.JSON404 == nil) {
				return err
			}
		}

		return nil
	}
}
