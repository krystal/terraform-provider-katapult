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
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
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
	_, ok := resp.Schema.Blocks["timeouts"].(resourceschema.SingleNestedBlock)
	require.True(t, ok)
}

func TestDiskResourceCreateRejectsMalformedSuccessResponse(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

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
	}

	require.NoError(t, r.diskRead(context.Background(), "disk_test", &model))
	require.True(t, model.BusType.IsNull())
	require.True(t, model.IOProfileID.IsNull())
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
					PostApplyPostRefresh: []plancheck.PlanCheck{
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
