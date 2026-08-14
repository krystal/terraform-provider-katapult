package v6provider

import (
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/next/core"
	"github.com/stretchr/testify/require"
)

func TestAccKatapultVirtualMachine_update_package(t *testing.T) {
	tt := newTestTools(t)
	name := tt.ResourceName()
	var vmID string

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckKatapultVirtualMachineDestroy(tt),
			testAccCheckKatapultIPDestroy(tt),
		),
		Steps: []resource.TestStep{
			{
				Config: virtualMachinePackageTestConfig(name, "rock-1"),
				Check:  virtualMachinePackageTestChecks("rock-1", &vmID, false),
			},
			{
				Config: virtualMachinePackageTestConfig(name, "rock-3"),
				Check:  virtualMachinePackageTestChecks("rock-3", &vmID, true),
			},
		},
	})
}

func TestAccKatapultVirtualMachine_update_package_by_id(t *testing.T) {
	tt := newTestTools(t)
	name := tt.ResourceName()
	var vmID string

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckKatapultVirtualMachineDestroy(tt),
			testAccCheckKatapultIPDestroy(tt),
		),
		Steps: []resource.TestStep{
			{
				Config: virtualMachinePackageTestConfig(
					name,
					"vmpkg_dhG25G5SX3HrA5j5",
				),
				Check: virtualMachinePackageTestChecks(
					"vmpkg_dhG25G5SX3HrA5j5",
					&vmID,
					false,
				),
			},
			{
				Config: virtualMachinePackageTestConfig(
					name,
					"vmpkg_Eh5LYVKScVHpj7sM",
				),
				Check: virtualMachinePackageTestChecks(
					"vmpkg_Eh5LYVKScVHpj7sM",
					&vmID,
					true,
				),
			},
		},
	})
}

func TestAccKatapultVirtualMachine_update_package_downgrade_error(
	t *testing.T,
) {
	tt := newTestTools(t)
	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckKatapultVirtualMachineDestroy(tt),
			testAccCheckKatapultIPDestroy(tt),
		),
		Steps: []resource.TestStep{
			{
				Config: virtualMachinePackageTestConfig(name, "rock-3"),
				Check: resource.TestCheckResourceAttr(
					"katapult_virtual_machine.base",
					"package",
					"rock-3",
				),
			},
			{
				Config: virtualMachinePackageTestConfig(name, "rock-1"),
				ExpectError: regexp.MustCompile(
					"cannot downgrade package unless the Virtual Machine is already",
				),
			},
		},
	})
}

func TestAccKatapultVirtualMachine_update_package_downgrade_stopped(
	t *testing.T,
) {
	tt := newTestTools(t)
	name := tt.ResourceName()
	var vmID string

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckKatapultVirtualMachineDestroy(tt),
			testAccCheckKatapultIPDestroy(tt),
		),
		Steps: []resource.TestStep{
			{
				Config: virtualMachinePackageTestConfig(name, "rock-3"),
				Check:  virtualMachinePackageTestChecks("rock-3", &vmID, false),
			},
			{
				PreConfig: func() {
					stopVirtualMachineForPackageTest(tt, vmID)
				},
				Config: virtualMachinePackageTestConfig(name, "rock-1"),
				Check:  virtualMachinePackageTestChecks("rock-1", &vmID, true),
			},
		},
	})
}

func TestAccKatapultVirtualMachine_update_package_downgrade_powered_off(
	t *testing.T,
) {
	tt := newTestTools(t)
	name := tt.ResourceName("package-downgrade-power")
	var vmID string

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckKatapultVirtualMachineDestroy(tt),
			testAccCheckKatapultIPDestroy(tt),
		),
		Steps: []resource.TestStep{
			{
				Config: virtualMachineManagedPackageTestConfig(
					name,
					"rock-3",
					true,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					virtualMachinePackageTestChecks("rock-3", &vmID, false),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base", "state", "started",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base", "powered_on", "true",
					),
				),
			},
			{
				Config: virtualMachineManagedPackageTestConfig(
					name,
					"rock-1",
					false,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					virtualMachinePackageTestChecks("rock-1", &vmID, true),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base", "state", "stopped",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base", "powered_on", "false",
					),
				),
			},
			{
				Config: virtualMachineManagedPackageTestConfig(
					name,
					"rock-1",
					true,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					virtualMachinePackageTestChecks("rock-1", &vmID, true),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base", "state", "started",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base", "powered_on", "true",
					),
				),
			},
		},
	})
}

func TestAccKatapultVirtualMachine_update_package_upgrade_stopped(
	t *testing.T,
) {
	tt := newTestTools(t)
	name := tt.ResourceName()
	var vmID string

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckKatapultVirtualMachineDestroy(tt),
			testAccCheckKatapultIPDestroy(tt),
		),
		Steps: []resource.TestStep{
			{
				Config: virtualMachinePackageTestConfig(name, "rock-1"),
				Check:  virtualMachinePackageTestChecks("rock-1", &vmID, false),
			},
			{
				PreConfig: func() {
					stopVirtualMachineForPackageTest(tt, vmID)
				},
				Config: virtualMachinePackageTestConfig(name, "rock-3"),
				Check:  virtualMachinePackageTestChecks("rock-3", &vmID, true),
			},
		},
	})
}

func virtualMachinePackageTestConfig(name string, pkg string) string {
	return undent.Stringf(`
		resource "katapult_ip" "web" {}

		resource "katapult_virtual_machine" "base" {
			name          = "%s"
			hostname      = "%s"
			package       = "%s"
			disk_template = "ubuntu-18-04"
			disk_template_options = {
				install_agent = true
			}
			ip_address_ids = [katapult_ip.web.id]

			timeouts {
				update = "10m"
			}
		}`,
		name,
		name+"-host",
		pkg,
	)
}

func virtualMachineManagedPackageTestConfig(
	name string,
	pkg string,
	poweredOn bool,
) string {
	return undent.Stringf(`
		resource "katapult_ip" "web" {}

		resource "katapult_virtual_machine" "base" {
			name          = "%s"
			hostname      = "%s"
			package       = "%s"
			powered_on    = %t
			disk_template = "ubuntu-18-04"
			disk_template_options = {
				install_agent = true
			}
			ip_address_ids = [katapult_ip.web.id]

			timeouts {
				update = "10m"
			}
		}`,
		name,
		name+"-host",
		pkg,
		poweredOn,
	)
}

func virtualMachinePackageTestChecks(
	pkg string,
	vmID *string,
	wantExistingID bool,
) resource.TestCheckFunc {
	idCheck := captureResourceAttr(
		"katapult_virtual_machine.base",
		"id",
		vmID,
	)
	if wantExistingID {
		idCheck = resource.TestCheckResourceAttrPtr(
			"katapult_virtual_machine.base",
			"id",
			vmID,
		)
	}

	return resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckResourceAttr(
			"katapult_virtual_machine.base",
			"package",
			pkg,
		),
		idCheck,
	)
}

func captureResourceAttr(
	resourceName string,
	attributeName string,
	target *string,
) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		resourceState, ok := state.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found: %s", resourceName)
		}

		value, ok := resourceState.Primary.Attributes[attributeName]
		if !ok {
			return fmt.Errorf(
				"attribute not found: %s.%s",
				resourceName,
				attributeName,
			)
		}

		*target = value

		return nil
	}
}

func stopVirtualMachineForPackageTest(tt *testTools, vmID string) {
	tt.T.Helper()

	stopRes, err := tt.Meta.Core.PostVirtualMachineStopWithResponse(
		tt.Ctx,
		core.PostVirtualMachineStopJSONRequestBody{
			VirtualMachine: core.VirtualMachineLookup{Id: &vmID},
		},
	)
	require.NoError(tt.T, err, "failed to stop virtual machine")
	require.NotNil(tt.T, stopRes, "stop response is missing")
	require.NotNil(tt.T, stopRes.JSON200, "stop response body is missing")
	require.NotNil(tt.T, stopRes.JSON200.Task.Id, "stop task ID is missing")
	err = waitForTaskCompletion(
		tt.Ctx,
		tt.Meta,
		5*time.Minute,
		*stopRes.JSON200.Task.Id,
	)
	require.NoError(tt.T, err, "stop virtual machine task failed")

	err = waitForVMToStop(tt.Ctx, tt.Meta, vmID, 5*time.Minute)
	require.NoError(tt.T, err, "virtual machine did not reach stopped state")
}
