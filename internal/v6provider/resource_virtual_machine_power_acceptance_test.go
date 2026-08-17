package v6provider

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
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
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: virtualMachinePowerStablePlanChecks(),
				},
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
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: virtualMachinePowerStablePlanChecks(),
				},
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

func virtualMachinePowerStablePlanChecks() []plancheck.PlanCheck {
	const address = "katapult_virtual_machine.base"

	return []plancheck.PlanCheck{
		plancheck.ExpectUnknownValue(address, tfjsonpath.New("state")),
		plancheck.ExpectKnownValue(
			address, tfjsonpath.New("fqdn"), knownvalue.NotNull(),
		),
		plancheck.ExpectKnownValue(
			address, tfjsonpath.New("ip_addresses"), knownvalue.SetSizeExact(1),
		),
		plancheck.ExpectKnownValue(
			address, tfjsonpath.New("network_interfaces"),
			knownvalue.ListSizeExact(1),
		),
		plancheck.ExpectKnownValue(
			address, tfjsonpath.New("system_disk").AtMapKey("state"),
			knownvalue.StringExact("built"),
		),
		plancheck.ExpectKnownValue(
			address, tfjsonpath.New("tags"), knownvalue.SetSizeExact(0),
		),
		plancheck.ExpectKnownValue(
			address, tfjsonpath.New("virtual_network_ids"),
			knownvalue.SetSizeExact(0),
		),
		expectKnownNullPlanValue{
			resourceAddress: address,
			attributePath:   tfjsonpath.New("group_id"),
		},
	}
}

type expectKnownNullPlanValue struct {
	resourceAddress string
	attributePath   tfjsonpath.Path
}

func (e expectKnownNullPlanValue) CheckPlan(
	_ context.Context,
	req plancheck.CheckPlanRequest,
	resp *plancheck.CheckPlanResponse,
) {
	for _, change := range req.Plan.ResourceChanges {
		if change.Address != e.resourceAddress {
			continue
		}
		unknown, unknownErr := tfjsonpath.Traverse(
			change.Change.AfterUnknown, e.attributePath,
		)
		if unknownErr == nil && unknown == true {
			resp.Error = fmt.Errorf(
				"expected known null value at %s.%s, but it was unknown",
				e.resourceAddress, e.attributePath.String(),
			)
			return
		}
		value, err := tfjsonpath.Traverse(change.Change.After, e.attributePath)
		if err != nil {
			resp.Error = err
			return
		}
		if value != nil {
			resp.Error = fmt.Errorf(
				"expected null value at %s.%s, got %v",
				e.resourceAddress, e.attributePath.String(), value,
			)
		}
		return
	}

	resp.Error = fmt.Errorf(
		"resource %s not found in plan", e.resourceAddress,
	)
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
			system_disk   = {}
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
