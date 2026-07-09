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
	"github.com/krystal/go-katapult"
	"github.com/krystal/go-katapult/core"
	"github.com/stretchr/testify/assert"
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
					resource "katapult_legacy_ip" "web" {}

					resource "katapult_virtual_machine" "base" {
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_legacy_ip.web.id]
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
						"katapult_legacy_ip.web", "id",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_virtual_machine.base", "ip_addresses.*",
						"katapult_legacy_ip.web", "address",
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

	name := tt.ResourceName()

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
					resource "katapult_legacy_ip" "web" {}
					resource "katapult_legacy_ip" "internal" {}

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
							katapult_legacy_ip.web.id,
							katapult_legacy_ip.internal.id
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
						"katapult_legacy_ip.web", "id",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_virtual_machine.base", "ip_address_ids.*",
						"katapult_legacy_ip.internal", "id",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"ip_addresses.#", "2",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_virtual_machine.base", "ip_addresses.*",
						"katapult_legacy_ip.web", "address",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_virtual_machine.base", "ip_addresses.*",
						"katapult_legacy_ip.internal", "address",
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

// TODO: Update this test to verify that the VM was created with the correct set
// of disks when go-katapult API client gains support for fetching VM disk
// details.
func TestAccKatapultVirtualMachine_custom_disks(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

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
					resource "katapult_legacy_ip" "web" {}
					resource "katapult_legacy_ip" "internal" {}

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
							katapult_legacy_ip.web.id,
							katapult_legacy_ip.internal.id
						]
						tags = ["web", "public"]
						network_speed_profile = "1gbps"
						disk {
							name = "System"
							size = 20
						}
						disk {
							name = "Data"
							size = 10
						}
						disk { # Automatically named "Disk #3"
							size = 10
						}
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
						"katapult_legacy_ip.web", "id",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_virtual_machine.base", "ip_address_ids.*",
						"katapult_legacy_ip.internal", "id",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"ip_addresses.#", "2",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_virtual_machine.base", "ip_addresses.*",
						"katapult_legacy_ip.web", "address",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_virtual_machine.base", "ip_addresses.*",
						"katapult_legacy_ip.internal", "address",
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

	name := tt.ResourceName()

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
					resource "katapult_legacy_ip" "web" {}

					resource "katapult_virtual_machine" "base" {
						name          = "%s"
						hostname      = "%s"
						description   = "A web server."
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_legacy_ip.web.id]
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
					resource "katapult_legacy_ip" "web" {}
					resource "katapult_virtual_machine" "base" {
						name          = "%s"
						hostname      = "%s"
						description   = "A app server."
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_legacy_ip.web.id]
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

func TestAccKatapultVirtualMachine_update_package(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	// Capture the VM ID across steps to assert the package change resizes
	// the VM in place rather than destroying and recreating it.
	var vmID string

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
					resource "katapult_legacy_ip" "web" {}

					resource "katapult_virtual_machine" "base" {
						name          = "%s"
						hostname      = "%s"
						package       = "rock-1"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_legacy_ip.web.id]
					}`,
					name, name+"-host",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVirtualMachineExists(
						tt, "katapult_virtual_machine.base",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"package", "rock-1",
					),
					extractResourceAttr(
						"katapult_virtual_machine.base", "id", &vmID,
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_legacy_ip" "web" {}

					resource "katapult_virtual_machine" "base" {
						name          = "%s"
						hostname      = "%s"
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_legacy_ip.web.id]
					}`,
					name, name+"-host",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVirtualMachineExists(
						tt, "katapult_virtual_machine.base",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"package", "rock-3",
					),
					// The VM must keep the same ID; a package change is an
					// in-place resize, not a destroy/recreate.
					resource.TestCheckResourceAttrPtr(
						"katapult_virtual_machine.base", "id", &vmID,
					),
				),
			},
		},
	})
}

func TestAccKatapultVirtualMachine_update_package_by_id(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	// Capture the VM ID across steps to assert the package change resizes
	// the VM in place rather than destroying and recreating it.
	var vmID string

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
					resource "katapult_legacy_ip" "web" {}

					resource "katapult_virtual_machine" "base" {
						name          = "%s"
						hostname      = "%s"
						package       = "vmpkg_dhG25G5SX3HrA5j5"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_legacy_ip.web.id]
					}`,
					name, name+"-host",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVirtualMachineExists(
						tt, "katapult_virtual_machine.base",
					),
					// State mirrors the format used in config: ID in, ID out.
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"package", "vmpkg_dhG25G5SX3HrA5j5",
					),
					extractResourceAttr(
						"katapult_virtual_machine.base", "id", &vmID,
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_legacy_ip" "web" {}

					resource "katapult_virtual_machine" "base" {
						name          = "%s"
						hostname      = "%s"
						package       = "vmpkg_Eh5LYVKScVHpj7sM"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_legacy_ip.web.id]
					}`,
					name, name+"-host",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVirtualMachineExists(
						tt, "katapult_virtual_machine.base",
					),
					// State mirrors the format used in config: ID in, ID out.
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"package", "vmpkg_Eh5LYVKScVHpj7sM",
					),
					// The VM must keep the same ID; a package change is an
					// in-place resize, not a destroy/recreate.
					resource.TestCheckResourceAttrPtr(
						"katapult_virtual_machine.base", "id", &vmID,
					),
				),
			},
		},
	})
}

func TestAccKatapultVirtualMachine_update_package_downgrade_error(t *testing.T) { //nolint:lll
	tt := newTestTools(t)

	name := tt.ResourceName()

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
					resource "katapult_legacy_ip" "web" {}

					resource "katapult_virtual_machine" "base" {
						name          = "%s"
						hostname      = "%s"
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_legacy_ip.web.id]
					}`,
					name, name+"-host",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVirtualMachineExists(
						tt, "katapult_virtual_machine.base",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"package", "rock-3",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_legacy_ip" "web" {}

					resource "katapult_virtual_machine" "base" {
						name          = "%s"
						hostname      = "%s"
						package       = "rock-1"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_legacy_ip.web.id]
					}`,
					name, name+"-host",
				),
				ExpectError: regexp.MustCompile(
					"cannot downgrade package while Virtual Machine is running",
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
					resource "katapult_legacy_ip" "web" {}

					resource "katapult_virtual_machine" "base" {
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [
							katapult_legacy_ip.web.id,
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
						"katapult_legacy_ip.web", "id",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"ip_addresses.#", "1",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_virtual_machine.base", "ip_addresses.*",
						"katapult_legacy_ip.web", "address",
					),
				),
			},
			{
				Config: undent.String(`
					resource "katapult_legacy_ip" "web" {}
					resource "katapult_legacy_ip" "office" {}

					resource "katapult_virtual_machine" "base" {
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [
							katapult_legacy_ip.web.id,
							katapult_legacy_ip.office.id,
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
						"katapult_legacy_ip.web", "id",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_virtual_machine.base", "ip_address_ids.*",
						"katapult_legacy_ip.office", "id",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"ip_addresses.#", "2",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_virtual_machine.base", "ip_addresses.*",
						"katapult_legacy_ip.web", "address",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_virtual_machine.base", "ip_addresses.*",
						"katapult_legacy_ip.office", "address",
					),
				),
			},
			{
				Config: undent.String(`
					resource "katapult_legacy_ip" "web" {}
					resource "katapult_legacy_ip" "office" {}

					resource "katapult_virtual_machine" "base" {
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [
							katapult_legacy_ip.web.id
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
						"katapult_legacy_ip.web", "id",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"ip_addresses.#", "1",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_virtual_machine.base", "ip_addresses.*",
						"katapult_legacy_ip.web", "address",
					),
				),
			},
		},
	})
}

func TestAccKatapultVirtualMachine_update_group(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

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
					resource "katapult_legacy_ip" "web" {}

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
							katapult_legacy_ip.web.id,
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
					resource "katapult_legacy_ip" "web" {}

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
							katapult_legacy_ip.web.id,
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
					resource "katapult_legacy_ip" "web" {}

					resource "katapult_virtual_machine" "base" {
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [
							katapult_legacy_ip.web.id,
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

	name := tt.ResourceName()

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
					resource "katapult_legacy_ip" "web" {}

					resource "katapult_virtual_machine" "base" {
						name          = "%s"
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [
							katapult_legacy_ip.web.id,
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
					resource "katapult_legacy_ip" "web" {}

					resource "katapult_virtual_machine" "base" {
						name          = "%s"
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [
							katapult_legacy_ip.web.id,
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
					resource "katapult_legacy_ip" "web" {}

					resource "katapult_virtual_machine" "base" {
						name          = "%s"
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [
							katapult_legacy_ip.web.id,
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

// extractResourceAttr captures the value of a resource attribute into target
// so it can be compared against in a later test step.
func extractResourceAttr(
	res string,
	attr string,
	target *string,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("resource not found: %s", res)
		}

		value, ok := rs.Primary.Attributes[attr]
		if !ok {
			return fmt.Errorf("attribute not found: %s.%s", res, attr)
		}

		*target = value

		return nil
	}
}

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
			if !errors.Is(err, katapult.ErrNotFound) {
				if err != nil {
					return err
				}

				return fmt.Errorf(
					"katapult_virtual_machine %s (%s) was not destroyed",
					rs.Primary.ID, vm.Name,
				)
			}

			_, _, err = m.Core.TrashObjects.GetByObjectID(
				tt.Ctx, rs.Primary.ID,
			)
			if !errors.Is(err, katapult.ErrNotFound) {
				if err != nil {
					return err
				}

				return fmt.Errorf(
					"katapult_virtual_machine %s was deleted, but not "+
						"purged from trash",
					rs.Primary.ID,
				)
			}
		}

		return nil
	}
}

func Test_normalizeVirtualMachinePackageForState(t *testing.T) {
	pkg := &core.VirtualMachinePackage{
		ID:        "vmpkg_YlqvfsKqZm6mpY4Y",
		Permalink: "rock-3",
	}

	tests := []struct {
		name       string
		configured string
		pkg        *core.VirtualMachinePackage
		want       string
	}{
		{
			name:       "nil package",
			configured: "rock-3",
			pkg:        nil,
			want:       "",
		},
		{
			name:       "configured with ID",
			configured: "vmpkg_YlqvfsKqZm6mpY4Y",
			pkg:        pkg,
			want:       "vmpkg_YlqvfsKqZm6mpY4Y",
		},
		{
			name:       "configured with permalink",
			configured: "rock-3",
			pkg:        pkg,
			want:       "rock-3",
		},
		{
			name:       "not configured (import)",
			configured: "",
			pkg:        pkg,
			want:       "rock-3",
		},
		{
			name:       "configured with ID, package has no ID",
			configured: "vmpkg_YlqvfsKqZm6mpY4Y",
			pkg:        &core.VirtualMachinePackage{Permalink: "rock-3"},
			want:       "rock-3",
		},
		{
			name:       "package has no permalink",
			configured: "rock-3",
			pkg: &core.VirtualMachinePackage{
				ID: "vmpkg_YlqvfsKqZm6mpY4Y",
			},
			want: "vmpkg_YlqvfsKqZm6mpY4Y",
		},
		{
			name:       "package has no ID or permalink",
			configured: "rock-3",
			pkg:        &core.VirtualMachinePackage{},
			want:       "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeVirtualMachinePackageForState(
				tt.configured, tt.pkg,
			)

			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_virtualMachinePackageRef(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  core.VirtualMachinePackageRef
	}{
		{
			name:  "ID",
			value: "vmpkg_YlqvfsKqZm6mpY4Y",
			want: core.VirtualMachinePackageRef{
				ID: "vmpkg_YlqvfsKqZm6mpY4Y",
			},
		},
		{
			name:  "permalink",
			value: "rock-3",
			want:  core.VirtualMachinePackageRef{Permalink: "rock-3"},
		},
		{
			name:  "empty",
			value: "",
			want:  core.VirtualMachinePackageRef{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := virtualMachinePackageRef(tt.value)

			assert.Equal(t, tt.want, got)
		})
	}
}
