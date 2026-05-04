package v6provider

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
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

	for _, vmg := range res.JSON200.VirtualMachineGroups {
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

func TestAccKatapultVMGroup_minimal(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultVMGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_virtual_machine_group" "minimal" {
						name = "%s"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVMGroupExists(
						tt, "katapult_virtual_machine_group.minimal",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine_group.minimal",
						"name", name,
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine_group.minimal",
						"segregate", "true",
					),
				),
			},
			{
				ResourceName:      "katapult_virtual_machine_group.minimal",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultVMGroup_segregated(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultVMGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_virtual_machine_group" "segregated" {
						name = "%s"
						segregate = true
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVMGroupExists(
						tt, "katapult_virtual_machine_group.segregated",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine_group.segregated",
						"segregate", "true",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine_group.segregated",
						"name", name,
					),
				),
			},
			{
				ResourceName:      "katapult_virtual_machine_group.segregated",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultVMGroup_not_segregated(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultVMGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_virtual_machine_group" "not-segregated" {
						name = "%s"
						segregate = false
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVMGroupExists(
						tt, "katapult_virtual_machine_group.not-segregated",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine_group.not-segregated",
						"segregate", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine_group.not-segregated",
						"name", name,
					),
				),
			},
			{
				//nolint:lll
				ResourceName:      "katapult_virtual_machine_group.not-segregated",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultVMGroup_update(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultVMGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_virtual_machine_group" "update" {
						name = "%s"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVMGroupExists(
						tt, "katapult_virtual_machine_group.update",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine_group.update",
						"segregate", "true",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine_group.update",
						"name", name,
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_virtual_machine_group" "update" {
						name = "%s - updated"
						segregate = false
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVMGroupExists(
						tt, "katapult_virtual_machine_group.update",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine_group.update",
						"segregate", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine_group.update",
						"name", name+" - updated",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_virtual_machine_group" "update" {
						name = "%s"
						segregate = true
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVMGroupExists(
						tt, "katapult_virtual_machine_group.update",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine_group.update",
						"segregate", "true",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine_group.update",
						"name", name,
					),
				),
			},
			{
				ResourceName:      "katapult_virtual_machine_group.update",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

//
// Helpers
//

func testAccCheckKatapultVMGroupExists(
	tt *testTools,
	res string,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("resource not found: %s", res)
		}

		resp, err := tt.Meta.Core.GetVirtualMachineGroupWithResponse(
			tt.Ctx,
			&core.GetVirtualMachineGroupParams{
				VirtualMachineGroupId: &rs.Primary.ID,
			},
		)
		if err != nil {
			return err
		}

		vmg := resp.JSON200.VirtualMachineGroup

		return resource.TestCheckResourceAttr(res, "id", *vmg.Id)(s)
	}
}

func testAccCheckKatapultVMGroupDestroy(
	tt *testTools,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "katapult_virtual_machine_group" {
				continue
			}

			resp, err := tt.Meta.Core.GetVirtualMachineGroupWithResponse(
				tt.Ctx,
				&core.GetVirtualMachineGroupParams{
					VirtualMachineGroupId: &rs.Primary.ID,
				},
			)

			if err == nil && resp.JSON200 != nil {
				return fmt.Errorf(
					"katapult_virtual_machine_group %s (%s) was not destroyed",
					rs.Primary.ID, *resp.JSON200.VirtualMachineGroup.Name,
				)
			}
		}

		return nil
	}
}
