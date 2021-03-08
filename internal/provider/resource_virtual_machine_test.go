package provider

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/pkg/katapult"
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

	var vms []*katapult.VirtualMachine
	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		pageResult, resp, err := m.Client.VirtualMachines.List(
			ctx, m.OrganizationRef(), &katapult.ListOptions{Page: pageNum},
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

		vm, _, err := m.Client.VirtualMachines.GetByID(ctx, vmSlim.ID)
		if err != nil {
			return err
		}

		log.Printf(
			"[DEBUG]  - Deleting Virtual Machine %s (%s)\n", vm.ID, vm.Name,
		)

		switch vm.State {
		case katapult.VirtualMachineStarted:
			err2 := stopVirtualMachine(ctx, m, 5*time.Minute, vm)
			if err2 != nil {
				return err2
			}
		case katapult.VirtualMachineStopping,
			katapult.VirtualMachineShuttingDown:
			vmWaiter := &resource.StateChangeConf{
				Pending: []string{
					string(katapult.VirtualMachineStarted),
					string(katapult.VirtualMachineStopping),
					string(katapult.VirtualMachineShuttingDown),
				},
				Target: []string{
					string(katapult.VirtualMachineStopped),
				},
				Refresh: func() (interface{}, string, error) {
					v, _, err2 := m.Client.VirtualMachines.GetByID(
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

			log.Printf("[DEBUG]    - stopping %s...\n", vm.ID)
			_, err2 := vmWaiter.WaitForStateContext(ctx)
			if err2 != nil {
				return fmt.Errorf(
					"failed to shutdown virtual machine: %w", err2,
				)
			}
		case katapult.VirtualMachineStopped:
			// no action needed
		default:
			return fmt.Errorf(
				"cannot stop virtual machine in state: %s",
				string(vm.State),
			)
		}

		trash, _, err := m.Client.VirtualMachines.Delete(ctx, vm)
		if err != nil {
			return err
		}

		task, _, err := m.Client.TrashObjects.Purge(ctx, trash)
		if err != nil {
			return err
		}

		taskWaiter := &resource.StateChangeConf{
			Pending: []string{
				string(katapult.TaskPending),
				string(katapult.TaskRunning),
			},
			Target: []string{
				string(katapult.TaskCompleted),
			},
			Refresh: func() (interface{}, string, error) {
				t, _, e := m.Client.Tasks.Get(ctx, task.ID)
				if e != nil {
					return 0, "", e
				}
				if t.Status == katapult.TaskFailed {
					return 0, string(t.Status), errors.New("task failed")
				}

				return t, string(t.Status), nil
			},
			Timeout:                   5 * time.Minute,
			Delay:                     2 * time.Second,
			MinTimeout:                5 * time.Second,
			ContinuousTargetOccurence: 1,
		}

		log.Printf("[DEBUG]    - purging %s to purge...\n", vm.ID)
		_, err = taskWaiter.WaitForStateContext(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}

func TestAccKatapultVirtualMachine_minimal(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckKatapultVirtualMachineDestroy(tt),
			testAccCheckKatapultIPDestroy(tt),
		),
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					resource "katapult_ip" "web" {}

					resource "katapult_virtual_machine" "base" {
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_ip.web.id]
					}`,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVirtualMachineExists(
						tt, "katapult_virtual_machine.base",
					),
					testCheckGeneratedResourceName(
						"katapult_virtual_machine.base", "name",
					),
					testCheckGeneratedHostnameName(
						"katapult_virtual_machine.base", "hostname",
					),
					resource.TestMatchResourceAttr(
						"katapult_virtual_machine.base",
						"fqdn", regexp.MustCompile(
							fmt.Sprintf(
								`^%s-.+-.+-.+\..+$`,
								regexp.QuoteMeta(testAccResourceNamePrefix),
							),
						),
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"package", "rock-3",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"disk_template", "ubuntu-18-04",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"disk_template_options.install_agent", "true",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_virtual_machine.base", "ip_address_ids.*",
						"katapult_ip.web", "id",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_virtual_machine.base", "ip_addresses.*",
						"katapult_ip.web", "address",
					),
				),
			},
		},
	})
}

func TestAccKatapultVirtualMachine_basic(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName("basic")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckKatapultVirtualMachineDestroy(tt),
			testAccCheckKatapultIPDestroy(tt),
		),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_ip" "web" {}

					resource "katapult_virtual_machine_group" "web" {
						name = "%s"
					}

					resource "katapult_virtual_machine" "base" {
						name          = "%s"
						hostname      = "%s"
						description   = "A web server."
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						group_id       = katapult_virtual_machine_group.web.id
						ip_address_ids = [katapult_ip.web.id]
						tags = ["web", "public"]
					}`,
					name+"-group", name, name+"-host",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVirtualMachineExists(
						tt, "katapult_virtual_machine.base",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"name", name,
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"hostname", name+"-host",
					),
					resource.TestMatchResourceAttr(
						"katapult_virtual_machine.base",
						"fqdn", regexp.MustCompile(
							fmt.Sprintf(
								`^%s\..+$`,
								regexp.QuoteMeta(name+"-host"),
							),
						),
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"description", "A web server.",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"package", "rock-3",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"disk_template", "ubuntu-18-04",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"disk_template_options.install_agent", "true",
					),
					resource.TestCheckResourceAttrPair(
						"katapult_virtual_machine.base", "group_id",
						"katapult_virtual_machine_group.web", "id",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"ip_address_ids.#", "1",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_virtual_machine.base", "ip_address_ids.*",
						"katapult_ip.web", "id",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"ip_addresses.#", "1",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_virtual_machine.base", "ip_addresses.*",
						"katapult_ip.web", "address",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"tags.#", "2",
					),
					resource.TestCheckTypeSetElemAttr(
						"katapult_virtual_machine.base", "tags.*", "web",
					),
					resource.TestCheckTypeSetElemAttr(
						"katapult_virtual_machine.base", "tags.*", "public",
					),
				),
			},
		},
	})
}

func TestAccKatapultVirtualMachine_update(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName("update")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckKatapultVirtualMachineDestroy(tt),
			testAccCheckKatapultIPDestroy(tt),
		),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_ip" "web" {}

					resource "katapult_virtual_machine" "base" {
						name          = "%s"
						hostname      = "%s"
						description   = "A web server."
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_ip.web.id]
						tags = ["web", "public"]
					}`,
					name, name+"-host",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVirtualMachineExists(
						tt, "katapult_virtual_machine.base",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"name", name,
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"hostname", name+"-host",
					),
					resource.TestMatchResourceAttr(
						"katapult_virtual_machine.base",
						"fqdn", regexp.MustCompile(
							fmt.Sprintf(
								`^%s\..+$`,
								regexp.QuoteMeta(name+"-host"),
							),
						),
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"description", "A web server.",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base", "tags.#", "2",
					),
					resource.TestCheckTypeSetElemAttr(
						"katapult_virtual_machine.base", "tags.*", "web",
					),
					resource.TestCheckTypeSetElemAttr(
						"katapult_virtual_machine.base", "tags.*", "public",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_ip" "web" {}
					resource "katapult_virtual_machine" "base" {
						name          = "%s"
						hostname      = "%s"
						description   = "A app server."
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_ip.web.id]
						tags = ["web", "app", "lb"]
					}`,
					name+"-diff", name+"-host-diff",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVirtualMachineExists(
						tt, "katapult_virtual_machine.base",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"name", name+"-diff",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"hostname", name+"-host-diff",
					),
					resource.TestMatchResourceAttr(
						"katapult_virtual_machine.base",
						"fqdn", regexp.MustCompile(
							fmt.Sprintf(
								`^%s\..+$`,
								regexp.QuoteMeta(name+"-host-diff"),
							),
						),
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"description", "A app server.",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base", "tags.#", "3",
					),
					resource.TestCheckTypeSetElemAttr(
						"katapult_virtual_machine.base", "tags.*", "web",
					),
					resource.TestCheckTypeSetElemAttr(
						"katapult_virtual_machine.base", "tags.*", "app",
					),
					resource.TestCheckTypeSetElemAttr(
						"katapult_virtual_machine.base", "tags.*", "lb",
					),
				),
			},
		},
	})
}

func TestAccKatapultVirtualMachine_update_ips(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckKatapultVirtualMachineDestroy(tt),
			testAccCheckKatapultIPDestroy(tt),
		),
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					resource "katapult_ip" "web" {}

					resource "katapult_virtual_machine" "base" {
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [
							katapult_ip.web.id,
						]
					}`,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVirtualMachineExists(
						tt, "katapult_virtual_machine.base",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"ip_address_ids.#", "1",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_virtual_machine.base", "ip_address_ids.*",
						"katapult_ip.web", "id",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"ip_addresses.#", "1",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_virtual_machine.base", "ip_addresses.*",
						"katapult_ip.web", "address",
					),
				),
			},
			{
				Config: undent.String(`
					resource "katapult_ip" "web" {}
					resource "katapult_ip" "office" {}

					resource "katapult_virtual_machine" "base" {
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [
							katapult_ip.web.id,
							katapult_ip.office.id,
						]
					}`,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVirtualMachineExists(
						tt, "katapult_virtual_machine.base",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"ip_address_ids.#", "2",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_virtual_machine.base", "ip_address_ids.*",
						"katapult_ip.web", "id",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_virtual_machine.base", "ip_address_ids.*",
						"katapult_ip.office", "id",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"ip_addresses.#", "2",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_virtual_machine.base", "ip_addresses.*",
						"katapult_ip.web", "address",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_virtual_machine.base", "ip_addresses.*",
						"katapult_ip.office", "address",
					),
				),
			},
			{
				Config: undent.String(`
					resource "katapult_ip" "web" {}
					resource "katapult_ip" "office" {}

					resource "katapult_virtual_machine" "base" {
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [
							katapult_ip.web.id
						]
						tags = ["web", "app", "lb"]
					}`,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVirtualMachineExists(
						tt, "katapult_virtual_machine.base",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"ip_address_ids.#", "1",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_virtual_machine.base", "ip_address_ids.*",
						"katapult_ip.web", "id",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"ip_addresses.#", "1",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_virtual_machine.base", "ip_addresses.*",
						"katapult_ip.web", "address",
					),
				),
			},
		},
	})
}

func TestAccKatapultVirtualMachine_update_group(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName("update_group")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckKatapultVirtualMachineDestroy(tt),
			testAccCheckKatapultIPDestroy(tt),
		),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_ip" "web" {}

					resource "katapult_virtual_machine_group" "web" {
						name = "%s"
					}

					resource "katapult_virtual_machine" "base" {
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [
							katapult_ip.web.id,
						]
					}`, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVirtualMachineExists(
						tt, "katapult_virtual_machine.base",
					),
					resource.TestCheckNoResourceAttr(
						"katapult_virtual_machine.base",
						"group_id",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_ip" "web" {}

					resource "katapult_virtual_machine_group" "web" {
						name = "%s"
					}

					resource "katapult_virtual_machine" "base" {
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [
							katapult_ip.web.id,
						]
						group_id = katapult_virtual_machine_group.web.id
					}`, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVirtualMachineExists(
						tt, "katapult_virtual_machine.base",
					),
					resource.TestCheckResourceAttrPair(
						"katapult_virtual_machine.base", "group_id",
						"katapult_virtual_machine_group.web", "id",
					),
				),
			},
			{
				Config: undent.String(`
					resource "katapult_ip" "web" {}

					resource "katapult_virtual_machine" "base" {
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [
							katapult_ip.web.id,
						]
					}`,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVirtualMachineExists(
						tt, "katapult_virtual_machine.base",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"group_id", "",
					),
				),
			},
		},
	})
}

//
// Helpers
//

func testAccCheckKatapultVirtualMachineExists(
	tt *testTools,
	res string,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := tt.Meta.Client

		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("resource not found: %s", res)
		}

		_, _, err := c.VirtualMachines.GetByID(tt.Ctx, rs.Primary.ID)
		if err != nil {
			return err
		}

		return nil
	}
}

func testAccCheckKatapultVirtualMachineDestroy(
	tt *testTools,
) resource.TestCheckFunc {
	m := tt.Meta

	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "katapult_virtual_machine" {
				continue
			}

			vm, _, err := m.Client.VirtualMachines.GetByID(
				tt.Ctx, rs.Primary.ID,
			)
			if err == nil && vm != nil {
				return fmt.Errorf(
					"katapult_virtual_machine %s (%s) was not destroyed",
					rs.Primary.ID, vm.Name,
				)
			}
		}

		return nil
	}
}
