package v6provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/krystal/go-katapult/next/core"
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

	res, err := m.Core.GetOrganizationVirtualMachineGroupsWithResponse(ctx,
		&core.GetOrganizationVirtualMachineGroupsParams{
			OrganizationSubDomain: &m.confOrganization,
		})
	if err != nil {
		return err
	}

	vmgs := res.JSON200.VirtualMachineGroups

	for _, vmg := range vmgs {
		if !strings.HasPrefix(*vmg.Name, testAccResourceNamePrefix) {
			continue
		}

		m.Logger.Info(
			"deleting virtual machine group", "id", vmg.Id, "name", vmg.Name,
		)

		_, err := m.Core.DeleteVirtualMachineGroupWithResponse(ctx,
			core.DeleteVirtualMachineGroupJSONRequestBody{
				VirtualMachineGroup: core.VirtualMachineGroupLookup{
					Id: vmg.Id,
				},
			})
		if err != nil {
			return err
		}
	}

	return nil
}
