package v6provider

import (
	"fmt"
	"regexp"
	"testing"
	"time"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/next/core"
	"github.com/stretchr/testify/require"
)

func TestAccKatapultDisk_delete_guards(t *testing.T) {
	tt := newTestTools(t)
	name := tt.ResourceName()
	var vmID, diskID string

	vmConfig := undent.Stringf(`
		resource "katapult_ip" "web" {}

		resource "katapult_virtual_machine" "base" {
			name          = "%s"
			package       = "rock-3"
			disk_template = "ubuntu-18-04"
			disk_template_options = {
				install_agent = true
			}
			ip_address_ids = [katapult_ip.web.id]
		}`, name)
	diskConfig := undent.Stringf(`
		resource "katapult_disk" "data" {
			name       = "%s-data"
			size_in_gb = 20
		}`, name)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckKatapultVirtualMachineDestroy(tt),
			testAccCheckKatapultDiskDestroy(tt),
			testAccCheckKatapultIPDestroy(tt),
		),
		Steps: []resource.TestStep{
			{
				Config: vmConfig + "\n" + diskConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					captureResourceAttr("katapult_virtual_machine.base", "id", &vmID),
					captureResourceAttr("katapult_disk.data", "id", &diskID),
				),
			},
			{
				PreConfig: func() { assignDiskOutsideTerraform(tt, vmID, diskID) },
				Config:    vmConfig + "\n" + diskConfig,
				Check: func(*terraform.State) error {
					obs, err := readDiskAssignmentObservation(tt.Ctx, tt.Meta, vmID, diskID)
					if err != nil {
						return err
					}
					if !obs.assigned || obs.vmID != vmID {
						return fmt.Errorf("out-of-band disk assignment was not observed")
					}
					return nil
				},
			},
			{
				Config:      vmConfig,
				ExpectError: regexp.MustCompile(`still has a Virtual Machine assignment`),
			},
			{
				Config:      diskConfig,
				ExpectError: regexp.MustCompile(`still has non-boot disk relationships`),
			},
			{
				PreConfig: func() { removeDiskAssignmentOutsideTerraform(tt, vmID, diskID) },
				Config:    "terraform {}",
			},
		},
	})
}

func assignDiskOutsideTerraform(tt *testTools, vmID, diskID string) {
	tt.T.Helper()
	res, err := tt.Meta.Core.PostDiskAssignWithResponse(
		tt.Ctx,
		core.PostDiskAssignJSONRequestBody{
			Disk:           core.DiskLookup{Id: &diskID},
			VirtualMachine: core.VirtualMachineLookup{Id: &vmID},
		},
	)
	require.NoError(tt.T, err)
	require.NotNil(tt.T, res)
	require.NotNil(tt.T, res.JSON200)
	require.NoError(tt.T, reconcileDiskAssignment(
		tt.Ctx, tt.Meta, vmID, diskID, true, 5*time.Minute,
	))
}

func removeDiskAssignmentOutsideTerraform(tt *testTools, vmID, diskID string) {
	tt.T.Helper()
	r := &DiskAssignmentResource{M: tt.Meta}
	state := diskAssignmentTestState(tt.T, r, DiskAssignmentResourceModel{
		ID:               types.StringValue(assignmentID(vmID, diskID)),
		VirtualMachineID: types.StringValue(vmID),
		DiskID:           types.StringValue(diskID),
		Attached:         types.BoolValue(true),
		AttachOnBoot:     types.BoolValue(true),
		AttachmentState:  types.StringValue("attached"),
	})
	resp := frameworkresource.DeleteResponse{State: state}
	r.Delete(
		tt.Ctx, frameworkresource.DeleteRequest{State: state}, &resp,
	)
	require.False(tt.T, resp.Diagnostics.HasError(), resp.Diagnostics.Errors())
}
