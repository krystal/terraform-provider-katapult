package v6provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/krystal/go-katapult/next/core"
)

func init() { //nolint:gochecknoinits
	resource.AddTestSweepers("katapult_virtual_machine", &resource.Sweeper{
		Name: "katapult_virtual_machine",
		F:    testSweepVirtualMachines,
	})
}

func testSweepVirtualMachines(_ string) error {
	m := sweepMeta()
	ctx := context.TODO()

	var vms []core.GetOrganizationVirtualMachines200ResponseVirtualMachines
	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		res, err := m.Core.GetOrganizationVirtualMachinesWithResponse(ctx,
			&core.GetOrganizationVirtualMachinesParams{
				OrganizationSubDomain: &m.confOrganization,
				Page:                  &pageNum,
			})
		if err != nil {
			return err
		}

		totalPages = res.JSON200.Pagination.TotalPages.MustGet()
		vms = append(vms, res.JSON200.VirtualMachines...)
	}

	for _, vmSlim := range vms {
		if !strings.HasPrefix(*vmSlim.Name, testAccResourceNamePrefix) {
			continue
		}

		vmRes, err := m.Core.GetVirtualMachineWithResponse(ctx,
			&core.GetVirtualMachineParams{
				VirtualMachineId: vmSlim.Id,
			})
		if err != nil {
			return err
		}

		vm := vmRes.JSON200.VirtualMachine

		m.Logger.Info("deleting virtual machine", "id", vm.Id, "name", vm.Name)

		stopped := false
		switch *vm.State { //nolint:exhaustive
		case core.Started:
			_, stopErr := m.Core.PostVirtualMachineStopWithResponse(ctx,
				core.PostVirtualMachineStopJSONRequestBody{
					VirtualMachine: core.VirtualMachineLookup{
						Id: vm.Id,
					},
				})
			if stopErr != nil {
				return stopErr
			}

		case core.Stopping,
			core.ShuttingDown:
			// Wait for the VM to stop.
		case core.Stopped:
			stopped = true
		default:
			return fmt.Errorf(
				"cannot stop virtual machine in state: %s",
				string(*vm.State),
			)
		}

		if !stopped {
			stopWaiter := &retry.StateChangeConf{
				Pending: []string{
					string(core.Started),
					string(core.Stopping),
					string(core.ShuttingDown),
				},
				Target: []string{
					string(core.Stopped),
				},
				Refresh: func() (interface{}, string, error) {
					res, err2 := m.Core.GetVirtualMachineWithResponse(ctx,
						&core.GetVirtualMachineParams{
							VirtualMachineId: vm.Id,
						})

					if err2 != nil {
						return 0, "", err2
					}

					return res.JSON200.VirtualMachine,
						string(*res.JSON200.VirtualMachine.State),
						nil
				},
				Timeout:                   5 * time.Minute,
				Delay:                     2 * time.Second,
				MinTimeout:                5 * time.Second,
				ContinuousTargetOccurence: 1,
			}

			m.Logger.Info(
				"stopping virtual machine", "id", vm.Id, "name", vm.Name,
			)

			_, err = stopWaiter.WaitForStateContext(ctx)
			if err != nil {
				return fmt.Errorf(
					"failed to shutdown virtual machine: %w", err,
				)
			}
		}

		delRes, err := m.Core.DeleteVirtualMachineWithResponse(ctx,
			core.DeleteVirtualMachineJSONRequestBody{
				VirtualMachine: &core.VirtualMachineLookup{
					Id: vm.Id,
				},
			})
		// trash, _, err := m.Core.VirtualMachines.Delete(ctx, vm.Ref())
		if err != nil {
			return err
		}

		trashObject := delRes.JSON200.TrashObject

		_, err = m.Core.DeleteTrashObjectWithResponse(ctx,
			core.DeleteTrashObjectJSONRequestBody{
				TrashObject: core.TrashObjectLookup{
					Id: trashObject.Id,
				},
			})
		if err != nil {
			return err
		}

		trashWaiter := &retry.StateChangeConf{
			Pending: []string{"exists"},
			Target:  []string{"not_found"},
			Refresh: func() (interface{}, string, error) {
				_, e := m.Core.GetTrashObjectWithResponse(ctx,
					&core.GetTrashObjectParams{
						TrashObjectId: trashObject.Id,
					})
				if e != nil {
					return nil, "", e
				}

				return nil, "exists", nil
			},
			Timeout:                   5 * time.Minute,
			Delay:                     2 * time.Second,
			MinTimeout:                5 * time.Second,
			ContinuousTargetOccurence: 1,
		}

		m.Logger.Info(
			"purging virtual machine", "id", vm.Id, "name", vm.Name,
		)

		_, err = trashWaiter.WaitForStateContext(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}
