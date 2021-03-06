package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/jimeh/undent"
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

		switch vm.State { //nolint:exhaustive
		case core.VirtualMachineStarted:
			err2 := stopVirtualMachine(ctx, m, 5*time.Minute, vm)
			if err2 != nil {
				return err2
			}
		case core.VirtualMachineStopping,
			core.VirtualMachineShuttingDown:
			vmWaiter := &resource.StateChangeConf{
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

			_, err2 := vmWaiter.WaitForStateContext(ctx)
			if err2 != nil {
				return fmt.Errorf(
					"failed to shutdown virtual machine: %w", err2,
				)
			}
		case core.VirtualMachineStopped:
			// no action needed
		default:
			return fmt.Errorf(
				"cannot stop virtual machine in state: %s",
				string(vm.State),
			)
		}

		trash, _, err := m.Core.VirtualMachines.Delete(ctx, vm.Ref())
		if err != nil {
			return err
		}

		task, _, err := m.Core.TrashObjects.Purge(ctx, trash.Ref())
		if err != nil {
			return err
		}

		taskWaiter := &resource.StateChangeConf{
			Pending: []string{
				string(core.TaskPending),
				string(core.TaskRunning),
			},
			Target: []string{
				string(core.TaskCompleted),
			},
			Refresh: func() (interface{}, string, error) {
				t, _, e := m.Core.Tasks.Get(ctx, task.ID)
				if e != nil {
					return 0, "", e
				}
				if t.Status == core.TaskFailed {
					return 0, string(t.Status), errors.New("task failed")
				}

				return t, string(t.Status), nil
			},
			Timeout:                   5 * time.Minute,
			Delay:                     2 * time.Second,
			MinTimeout:                5 * time.Second,
			ContinuousTargetOccurence: 1,
		}

		m.Logger.Info(
			"purging virtual machine", "id", vm.ID, "name", vm.Name,
		)

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
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"network_speed_profile", "10gbps",
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
					resource "katapult_ip" "internal" {}

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
						ip_address_ids = [
							katapult_ip.web.id,
							katapult_ip.internal.id
						]
						tags = ["web", "public"]
						network_speed_profile = "1gbps"
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
						"ip_address_ids.#", "2",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_virtual_machine.base", "ip_address_ids.*",
						"katapult_ip.web", "id",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_virtual_machine.base", "ip_address_ids.*",
						"katapult_ip.internal", "id",
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
						"katapult_ip.internal", "address",
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
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"network_speed_profile", "1gbps",
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
						network_speed_profile = "1gbps"
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
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"network_speed_profile", "1gbps",
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
						network_speed_profile = "10gbps"
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
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"network_speed_profile", "10gbps",
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

func TestAccKatapultVirtualMachine_update_network_speed_profile(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName("update_speed_profile")

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
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"network_speed_profile", "10gbps",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_ip" "web" {}

					resource "katapult_virtual_machine" "base" {
						name          = "%s"
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [
							katapult_ip.web.id,
						]
						network_speed_profile = "10gbps"
					}`, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVirtualMachineExists(
						tt, "katapult_virtual_machine.base",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"network_speed_profile", "10gbps",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_ip" "web" {}

					resource "katapult_virtual_machine" "base" {
						name          = "%s"
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [
							katapult_ip.web.id,
						]
						network_speed_profile = "1gbps"
					}`, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVirtualMachineExists(
						tt, "katapult_virtual_machine.base",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"network_speed_profile", "1gbps",
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
		c := tt.Meta.Core

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

			vm, _, err := m.Core.VirtualMachines.GetByID(
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
