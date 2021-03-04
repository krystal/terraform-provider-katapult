package provider

import (
	"context"
	"fmt"
	"log"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/jimeh/undent"
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

	vmgs, _, err := m.Client.VirtualMachineGroups.List(
		ctx, m.OrganizationRef(),
	)
	if err != nil {
		return err
	}

	for _, vmg := range vmgs {
		if !strings.HasPrefix(vmg.Name, testAccResourceNamePrefix) {
			continue
		}

		log.Printf(
			"[DEBUG]  - Deleting Virtual Machine Group %s (%s)\n",
			vmg.ID, vmg.Name,
		)
		_, err := m.Client.VirtualMachineGroups.Delete(ctx, vmg)
		if err != nil {
			return err
		}
	}

	return nil
}

func TestAccKatapultVMGroup_minimal(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName("minimal")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultVMGroupDestroy(tt),
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
						"name",
						name,
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

	name := tt.ResourceName("segregated")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultVMGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_virtual_machine_group" "segregated" {
						segregate = true
						name = "%s"
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

	name := tt.ResourceName("not-segregated")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultVMGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_virtual_machine_group" "not-segregated" {
					  segregate = false
						name = "%s"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
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

	name := tt.ResourceName("update")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultVMGroupDestroy(tt),
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

func testAccCheckKatapultVMGroupDestroy(
	tt *testTools,
) resource.TestCheckFunc {
	m := tt.Meta

	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "katapult_virtual_machine_group" {
				continue
			}

			vmg, _, err := m.Client.VirtualMachineGroups.GetByID(
				tt.Ctx, rs.Primary.ID,
			)

			if err == nil && vmg != nil {
				return fmt.Errorf(
					"katapult_virtual_machine_group %s (%s) was not destroyed",
					rs.Primary.ID, vmg.Name,
				)
			}
		}

		return nil
	}
}

func testAccCheckKatapultVMGroupExists(
	tt *testTools,
	res string,
) resource.TestCheckFunc {
	m := tt.Meta

	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("resource not found: %s", res)
		}

		ip, _, err := m.Client.VirtualMachineGroups.GetByID(
			tt.Ctx, rs.Primary.ID,
		)
		if err != nil {
			return err
		}

		return resource.TestCheckResourceAttr(res, "id", ip.ID)(s)
	}
}
