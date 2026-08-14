package v6provider

import (
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/next/core"
	"github.com/stretchr/testify/require"
)

func TestAccKatapultVirtualMachine_power_state(t *testing.T) {
	tt := newTestTools(t)
	name := tt.ResourceName()
	poweredOn := true
	poweredOff := false
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
				Config: virtualMachinePowerTestConfig(name, &poweredOff),
				Check: virtualMachinePowerTestChecks(
					"stopped", false, &vmID, false,
				),
			},
			{
				Config: virtualMachinePowerTestConfig(name, &poweredOn),
				Check: virtualMachinePowerTestChecks(
					"started", true, &vmID, true,
				),
			},
			{
				PreConfig: func() {
					shutdownVirtualMachineForPowerTest(tt, vmID)
				},
				Config: virtualMachinePowerTestConfig(name, &poweredOn),
				Check: virtualMachinePowerTestChecks(
					"started", true, &vmID, true,
				),
			},
			{
				Config: virtualMachinePowerTestConfig(name, &poweredOff),
				Check: virtualMachinePowerTestChecks(
					"stopped", false, &vmID, true,
				),
			},
			{
				PreConfig: func() {
					startVirtualMachineForPowerTest(tt, vmID)
				},
				Config: virtualMachinePowerTestConfig(name, &poweredOff),
				Check: virtualMachinePowerTestChecks(
					"stopped", false, &vmID, true,
				),
			},
			{
				Config: virtualMachinePowerTestConfig(name, nil),
				Check: virtualMachinePowerTestChecks(
					"stopped", false, &vmID, true,
				),
			},
			{
				PreConfig: func() {
					startVirtualMachineForPowerTest(tt, vmID)
				},
				Config: virtualMachinePowerTestConfig(name, nil),
				Check: virtualMachinePowerTestChecks(
					"started", true, &vmID, true,
				),
			},
		},
	})
}

func virtualMachinePowerTestConfig(name string, poweredOn *bool) string {
	poweredOnConfig := ""
	if poweredOn != nil {
		poweredOnConfig = fmt.Sprintf("powered_on = %t", *poweredOn)
	}

	return undent.Stringf(`
		resource "katapult_ip" "web" {}

		resource "katapult_virtual_machine" "base" {
			name          = "%s"
			hostname      = "%s"
			package       = "rock-1"
			disk_template = "ubuntu-18-04"
			disk_template_options = {
				install_agent = true
			}
			ip_address_ids = [katapult_ip.web.id]
			%s

			timeouts {
				create = "20m"
				update = "10m"
				delete = "10m"
			}
		}`,
		name,
		name+"-host",
		poweredOnConfig,
	)
}

func virtualMachinePowerTestChecks(
	state string,
	poweredOn bool,
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
			"state",
			state,
		),
		resource.TestCheckResourceAttr(
			"katapult_virtual_machine.base",
			"powered_on",
			fmt.Sprintf("%t", poweredOn),
		),
		idCheck,
	)
}

func shutdownVirtualMachineForPowerTest(tt *testTools, vmID string) {
	tt.T.Helper()

	res, err := tt.Meta.Core.PostVirtualMachineShutdownWithResponse(
		tt.Ctx,
		core.PostVirtualMachineShutdownJSONRequestBody{
			VirtualMachine: core.VirtualMachineLookup{Id: &vmID},
		},
	)
	require.NoError(tt.T, err, "failed to shut down virtual machine")
	require.NotNil(tt.T, res, "shutdown response is missing")
	require.NotNil(tt.T, res.JSON200, "shutdown response body is missing")
	require.NotNil(tt.T, res.JSON200.Task.Id, "shutdown task ID is missing")
	waitForVirtualMachinePowerTest(tt, vmID, *res.JSON200.Task.Id, core.Stopped)
}

func startVirtualMachineForPowerTest(tt *testTools, vmID string) {
	tt.T.Helper()

	res, err := tt.Meta.Core.PostVirtualMachineStartWithResponse(
		tt.Ctx,
		core.PostVirtualMachineStartJSONRequestBody{
			VirtualMachine: core.VirtualMachineLookup{Id: &vmID},
		},
	)
	require.NoError(tt.T, err, "failed to start virtual machine")
	require.NotNil(tt.T, res, "start response is missing")
	require.NotNil(tt.T, res.JSON200, "start response body is missing")
	require.NotNil(tt.T, res.JSON200.Task.Id, "start task ID is missing")
	waitForVirtualMachinePowerTest(tt, vmID, *res.JSON200.Task.Id, core.Started)
}

func waitForVirtualMachinePowerTest(
	tt *testTools,
	vmID string,
	taskID string,
	target core.VirtualMachineStateEnum,
) {
	tt.T.Helper()

	err := waitForTaskCompletion(tt.Ctx, tt.Meta, 5*time.Minute, taskID)
	require.NoError(tt.T, err, "virtual machine power task failed")

	err = waitForVirtualMachineState(
		tt.Ctx,
		tt.Meta,
		vmID,
		virtualMachineExactStatePending(target),
		[]core.VirtualMachineStateEnum{target},
		5*time.Minute,
	)
	require.NoError(
		tt.T,
		err,
		"virtual machine did not reach %s state",
		target,
	)
}
