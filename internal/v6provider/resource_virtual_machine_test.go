package v6provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/krystal/go-katapult"
	"github.com/krystal/go-katapult/core"
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

	var vms []*core.VirtualMachine
	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		pageResult, resp, err := m.Core.VirtualMachines.List(
			ctx, m.OrganizationRef, &core.ListOptions{Page: pageNum},
		)
		if err != nil {
			return err
		}

		totalPages = resp.Pagination.TotalPages
		vms = append(vms, pageResult...)
	}

	for _, vmSlim := range vms {
		if !strings.HasPrefix(vmSlim.Name, testAccResourceNamePrefix) {
			continue
		}

		vm, _, err := m.Core.VirtualMachines.GetByID(ctx, vmSlim.ID)
		if err != nil {
			return err
		}

		m.Logger.Info("deleting virtual machine", "id", vm.ID, "name", vm.Name)

		stopped := false
		switch vm.State { //nolint:exhaustive
		case core.VirtualMachineStarted:
			_, _, err = m.Core.VirtualMachines.Stop(ctx, vm.Ref())
			if err != nil {
				return err
			}
		case core.VirtualMachineStopping,
			core.VirtualMachineShuttingDown:
			// Wait for the VM to stop.
		case core.VirtualMachineStopped:
			stopped = true
		default:
			return fmt.Errorf(
				"cannot stop virtual machine in state: %s",
				string(vm.State),
			)
		}

		if !stopped {
			stopWaiter := &resource.StateChangeConf{
				Pending: []string{
					string(core.VirtualMachineStarted),
					string(core.VirtualMachineStopping),
					string(core.VirtualMachineShuttingDown),
				},
				Target: []string{
					string(core.VirtualMachineStopped),
				},
				Refresh: func() (interface{}, string, error) {
					v, _, err2 := m.Core.VirtualMachines.GetByID(
						ctx, vm.ID,
					)
					if err2 != nil {
						return 0, "", err2
					}

					return v, string(v.State), nil
				},
				Timeout:                   5 * time.Minute,
				Delay:                     2 * time.Second,
				MinTimeout:                5 * time.Second,
				ContinuousTargetOccurence: 1,
			}

			m.Logger.Info(
				"stopping virtual machine", "id", vm.ID, "name", vm.Name,
			)

			_, err = stopWaiter.WaitForStateContext(ctx)
			if err != nil {
				return fmt.Errorf(
					"failed to shutdown virtual machine: %w", err,
				)
			}
		}

		trash, _, err := m.Core.VirtualMachines.Delete(ctx, vm.Ref())
		if err != nil {
			return err
		}

		trashRef := trash.Ref()
		_, _, err = m.Core.TrashObjects.Purge(ctx, trashRef)
		if err != nil {
			return err
		}

		trashWaiter := &resource.StateChangeConf{
			Pending: []string{"exists"},
			Target:  []string{"not_found"},
			Refresh: func() (interface{}, string, error) {
				_, _, e := m.Core.TrashObjects.Get(ctx, trashRef)
				if e != nil && errors.Is(e, katapult.ErrNotFound) {
					return 1, "not_found", nil
				}

				return nil, "exists", nil
			},
			Timeout:                   5 * time.Minute,
			Delay:                     2 * time.Second,
			MinTimeout:                5 * time.Second,
			ContinuousTargetOccurence: 1,
		}

		m.Logger.Info(
			"purging virtual machine", "id", vm.ID, "name", vm.Name,
		)

		_, err = trashWaiter.WaitForStateContext(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}