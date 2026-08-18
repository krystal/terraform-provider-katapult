package v6provider

import (
	"fmt"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/next/core"
	"github.com/stretchr/testify/require"
)

// TestAccKatapultVirtualMachine_disk_assignment_power_and_resize covers the
// power-dependent assignment contract and every supported growth mode against
// one disk. System-disk shrink is covered separately with a stopped VM.
func TestAccKatapultVirtualMachine_disk_assignment_power_and_resize(t *testing.T) {
	tt := newTestTools(t)
	name := tt.ResourceName()
	var vmID, diskID string

	config := func(poweredOn, attached bool, size int, method string) string {
		return diskAssignmentPowerResizeConfig(name, poweredOn, attached, size, method)
	}

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
				Config: config(false, true, 20, "offline"),
				Check: resource.ComposeAggregateTestCheckFunc(
					captureResourceAttr("katapult_virtual_machine.base", "id", &vmID),
					captureResourceAttr("katapult_disk.data", "id", &diskID),
					resource.TestCheckResourceAttr("katapult_virtual_machine.base", "state", "stopped"),
					resource.TestCheckResourceAttr("katapult_disk_assignment.data", "attached", "true"),
					resource.TestCheckResourceAttr("katapult_disk_assignment.data", "attach_on_boot", "true"),
					resource.TestCheckResourceAttr("katapult_disk_assignment.data", "attachment_state", "detached"),
					checkAssignedDiskDataSource("data.katapult_disk.data", "detached", true),
					checkVMDiskDataSourceEntry(
						"data.katapult_virtual_machine_disks.all",
						"katapult_disk.data", false, true, "detached", 20,
					),
				),
			},
			{
				ResourceName: "katapult_disk_assignment.data",
				ImportStateIdFunc: func(*terraform.State) (string, error) {
					return assignmentID(vmID, diskID), nil
				},
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
			{
				Config: config(true, true, 20, "offline"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPtr("katapult_virtual_machine.base", "id", &vmID),
					resource.TestCheckResourceAttrPtr("katapult_disk.data", "id", &diskID),
					resource.TestCheckResourceAttr("katapult_virtual_machine.base", "state", "started"),
				),
			},
			{
				PreConfig: func() {
					require.NoError(t, waitForDiskAttachmentForAcceptance(
						tt, vmID, diskID, core.VirtualMachineDiskAttachmentStateEnumAttached,
					))
				},
				Config: config(true, true, 20, "offline"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("katapult_disk_assignment.data", "attachment_state", "attached"),
					checkAssignedDiskDataSource("data.katapult_disk.data", "attached", true),
					checkAssignedDiskCollectionEntry("data.katapult_disks.all", "katapult_disk.data"),
				),
			},
			{
				Config: config(true, true, 30, "offline"),
				ExpectError: regexp.MustCompile(
					`offline growth requires physical detachment`,
				),
			},
			{
				Config: config(true, true, 30, "online"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("katapult_disk.data", "size_in_gb", "30"),
					resource.TestCheckResourceAttrPtr("katapult_disk.data", "id", &diskID),
					resource.TestCheckResourceAttr("katapult_disk_assignment.data", "attachment_state", "attached"),
				),
			},
			{
				Config: config(true, true, 20, "online"),
				ExpectError: regexp.MustCompile(
					`shrinking requires physical detachment`,
				),
			},
			{
				Config: config(true, false, 30, "online"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("katapult_disk_assignment.data", "attached", "false"),
					resource.TestCheckResourceAttr("katapult_disk_assignment.data", "attach_on_boot", "false"),
					resource.TestCheckResourceAttr("katapult_disk_assignment.data", "attachment_state", "detached"),
					checkAssignedDiskDataSource("data.katapult_disk.data", "detached", false),
				),
			},
			{
				// A configured online preference must fall back to an offline resize
				// once the assigned disk is physically detached.
				Config: config(true, false, 40, "online"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("katapult_disk.data", "size_in_gb", "40"),
					resource.TestCheckResourceAttrPtr("katapult_disk.data", "id", &diskID),
					resource.TestCheckResourceAttr("katapult_disk_assignment.data", "attachment_state", "detached"),
					checkVMDiskDataSourceEntry(
						"data.katapult_virtual_machine_disks.all",
						"katapult_disk.data", false, false, "detached", 40,
					),
				),
			},
			{
				PreConfig: func() {
					require.NoError(t, reconcileDiskAssignment(
						tt.Ctx, tt.Meta, vmID, diskID, true, 5*time.Minute,
					))
				},
				Config: config(true, false, 40, "online"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("katapult_disk_assignment.data", "attached", "false"),
					resource.TestCheckResourceAttr("katapult_disk_assignment.data", "attach_on_boot", "false"),
					resource.TestCheckResourceAttr("katapult_disk_assignment.data", "attachment_state", "detached"),
				),
			},
		},
	})
}

func waitForDiskAttachmentForAcceptance(
	tt *testTools,
	vmID, diskID string,
	target core.VirtualMachineDiskAttachmentStateEnum,
) error {
	waiter := &retry.StateChangeConf{
		Pending: []string{
			unknownStateValue,
			string(core.VirtualMachineDiskAttachmentStateEnumDetached),
			string(core.VirtualMachineDiskAttachmentStateEnumAttaching),
			string(core.VirtualMachineDiskAttachmentStateEnumDetaching),
		},
		Target:       []string{string(target)},
		Timeout:      5 * time.Minute,
		Delay:        tt.Meta.stateChangeDelay(time.Second),
		MinTimeout:   tt.Meta.stateChangeDelay(2 * time.Second),
		PollInterval: tt.Meta.stateChangePollInterval(),
		Refresh: func() (interface{}, string, error) {
			obs, err := readDiskAssignmentObservation(tt.Ctx, tt.Meta, vmID, diskID)
			if err != nil {
				return nil, "", err
			}
			if obs.attachmentState == nil {
				return obs, unknownStateValue, nil
			}
			return obs, string(*obs.attachmentState), nil
		},
	}
	_, err := waiter.WaitForStateContext(tt.Ctx)
	return err
}

func diskAssignmentPowerResizeConfig(
	name string,
	poweredOn, attached bool,
	size int,
	resizeMethod string,
) string {
	return undent.Stringf(`
		resource "katapult_ip" "web" {}

		resource "katapult_disk" "data" {
			name          = "%s-data"
			size_in_gb    = %d
			resize_method = "%s"
		}

		resource "katapult_virtual_machine" "base" {
			name          = "%s"
			package       = "rock-3"
			disk_template = "ubuntu-18-04"
			disk_template_options = {
				install_agent = true
			}
			ip_address_ids = [katapult_ip.web.id]
			powered_on     = %t
		}

		resource "katapult_disk_assignment" "data" {
			virtual_machine_id = katapult_virtual_machine.base.id
			disk_id            = katapult_disk.data.id
			attached           = %t
		}

		data "katapult_disk" "data" {
			id = katapult_disk.data.id
			depends_on = [
				katapult_disk.data,
				katapult_disk_assignment.data,
			]
		}

		data "katapult_virtual_machine_disks" "all" {
			virtual_machine_id = katapult_virtual_machine.base.id
			depends_on = [
				katapult_disk.data,
				data.katapult_disk.data,
			]
		}

		data "katapult_disks" "all" {
			depends_on = [
				katapult_disk.data,
				data.katapult_virtual_machine_disks.all,
			]
		}`, name, size, resizeMethod, name, poweredOn, attached)
}

func checkAssignedDiskDataSource(
	address, attachmentState string,
	attachOnBoot bool,
) resource.TestCheckFunc {
	return resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckResourceAttrPair(
			address, "virtual_machine_id", "katapult_virtual_machine.base", "id",
		),
		resource.TestCheckResourceAttrPair(
			address, "virtual_machine_fqdn", "katapult_virtual_machine.base", "fqdn",
		),
		resource.TestCheckResourceAttr(address, "boot", "false"),
		resource.TestCheckResourceAttr(
			address, "attach_on_boot", fmt.Sprintf("%t", attachOnBoot),
		),
		resource.TestCheckResourceAttr(address, "attachment_state", attachmentState),
	)
}

func checkAssignedDiskCollectionEntry(
	dataSourceAddress, diskResourceAddress string,
) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		dataSource, ok := state.RootModule().Resources[dataSourceAddress]
		if !ok {
			return fmt.Errorf("resource not found: %s", dataSourceAddress)
		}
		disk, ok := state.RootModule().Resources[diskResourceAddress]
		if !ok {
			return fmt.Errorf("resource not found: %s", diskResourceAddress)
		}
		vm := state.RootModule().Resources["katapult_virtual_machine.base"]
		if vm == nil {
			return fmt.Errorf("resource not found: katapult_virtual_machine.base")
		}
		count, err := strconv.Atoi(dataSource.Primary.Attributes["disks.#"])
		if err != nil {
			return fmt.Errorf("reading %s disk count: %w", dataSourceAddress, err)
		}
		for i := range count {
			prefix := fmt.Sprintf("disks.%d.", i)
			if dataSource.Primary.Attributes[prefix+"id"] != disk.Primary.ID {
				continue
			}
			if got := dataSource.Primary.Attributes[prefix+"virtual_machine_id"]; got != vm.Primary.ID {
				return fmt.Errorf("assigned VM ID = %q, want %q", got, vm.Primary.ID)
			}
			gotFQDN := dataSource.Primary.Attributes[prefix+"virtual_machine_fqdn"]
			if gotFQDN != vm.Primary.Attributes["fqdn"] {
				return fmt.Errorf(
					"assigned VM FQDN = %q, want %q",
					gotFQDN, vm.Primary.Attributes["fqdn"],
				)
			}
			return nil
		}
		return fmt.Errorf("disk %s not found in %s", disk.Primary.ID, dataSourceAddress)
	}
}
