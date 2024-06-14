package v6provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func init() { //nolint:gochecknoinits
	resource.AddTestSweepers("katapult_virtual_machine_group",
		&resource.Sweeper{
			Name: "katapult_virtual_machine_group",
			F:    testSweepVMGroups,
		})
}

func testSweepVMGroups(_ string) error {
	m := sweepMeta()
	ctx := context.TODO()

	vmgs, _, err := m.Core.VirtualMachineGroups.List(
		ctx, m.OrganizationRef,
	)
	if err != nil {
		return err
	}

	for _, vmg := range vmgs {
		if !strings.HasPrefix(vmg.Name, testAccResourceNamePrefix) {
			continue
		}

		m.Logger.Info(
			"deleting virtual machine group", "id", vmg.ID, "name", vmg.Name,
		)
		_, err := m.Core.VirtualMachineGroups.Delete(ctx, vmg.Ref())
		if err != nil {
			return err
		}
	}

	return nil
}
