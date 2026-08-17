package v6provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/next/core"
)

func init() { //nolint:gochecknoinits
	resource.AddTestSweepers("katapult_virtual_machine", &resource.Sweeper{
		Name: "katapult_virtual_machine",
		F:    testSweepVirtualMachines,
	})
}

//nolint:gocyclo // Keep the destructive stop, delete, and purge lifecycle linear and auditable.
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
		if vmSlim.Name == nil {
			continue
		}
		if !strings.HasPrefix(*vmSlim.Name, testAccResourceNamePrefix) {
			continue
		}
		if vmSlim.Id == nil {
			return fmt.Errorf("virtual machine %q has no ID", *vmSlim.Name)
		}

		vmRes, err := m.Core.GetVirtualMachineWithResponse(ctx,
			&core.GetVirtualMachineParams{
				VirtualMachineId: vmSlim.Id,
			})
		if err != nil {
			return err
		}
		if vmRes == nil || vmRes.JSON200 == nil {
			return fmt.Errorf("unexpected empty response fetching virtual machine %s", *vmSlim.Id)
		}

		vm := vmRes.JSON200.VirtualMachine
		if vm.Id == nil || vm.Name == nil || vm.State == nil {
			return fmt.Errorf("virtual machine %s response has incomplete identity or state", *vmSlim.Id)
		}
		vmID, vmName := *vm.Id, *vm.Name

		m.Logger.Info("deleting virtual machine", "id", vmID, "name", vmName)

		stopped := false
		switch *vm.State { //nolint:exhaustive
		case core.Started:
			_, stopErr := m.Core.PostVirtualMachineStopWithResponse(ctx,
				core.PostVirtualMachineStopJSONRequestBody{
					VirtualMachine: core.VirtualMachineLookup{
						Id: &vmID,
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
							VirtualMachineId: &vmID,
						})

					if err2 != nil {
						return 0, "", err2
					}

					return res.JSON200.VirtualMachine,
						string(*res.JSON200.VirtualMachine.State),
						nil
				},
				Timeout:                   5 * time.Minute,
				Delay:                     m.stateChangeDelay(2 * time.Second),
				MinTimeout:                m.stateChangeDelay(5 * time.Second),
				PollInterval:              m.stateChangePollInterval(),
				ContinuousTargetOccurence: 1,
			}

			m.Logger.Info(
				"stopping virtual machine", "id", vmID, "name", vmName,
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
					Id: &vmID,
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
				if e != nil && errors.Is(e, core.ErrNotFound) {
					return 1, "not_found", nil
				}

				return nil, "exists", nil
			},
			Timeout:                   5 * time.Minute,
			Delay:                     m.stateChangeDelay(2 * time.Second),
			MinTimeout:                m.stateChangeDelay(5 * time.Second),
			PollInterval:              m.stateChangePollInterval(),
			ContinuousTargetOccurence: 1,
		}

		m.Logger.Info(
			"purging virtual machine", "id", vmID, "name", vmName,
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
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
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

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
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
								`(?i)^%s\..+$`,
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

// TestAccKatapultVirtualMachine_custom_disks verifies the documented migration
// from deprecated nested disk blocks to first-class disk and assignment
// resources without replacing the VM or any disk.
func TestAccKatapultVirtualMachine_custom_disks(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()
	var vmID, bootDiskID, dataDiskAID, dataDiskBID string

	legacyConfig := undent.Stringf(`
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
			disk {
				name = "System"
				size = 20
			}
			disk {
				name = "Data A"
				size = 10
			}
			disk {
				name = "Data B"
				size = 10
			}
		}

		data "katapult_virtual_machine_disks" "all" {
			virtual_machine_id = katapult_virtual_machine.base.id
		}`, name, name+"-host")

	importConfig := legacyConfig + "\n" + undent.String(`
		resource "katapult_disk" "data_a" {
			name       = "Data A"
			size_in_gb = 10
		}

		resource "katapult_disk" "data_b" {
			name       = "Data B"
			size_in_gb = 10
		}

		resource "katapult_disk_assignment" "data_a" {
			virtual_machine_id = katapult_virtual_machine.base.id
			disk_id            = katapult_disk.data_a.id
		}

		resource "katapult_disk_assignment" "data_b" {
			virtual_machine_id = katapult_virtual_machine.base.id
			disk_id            = katapult_disk.data_b.id
		}
	`)

	firstClassConfig := undent.Stringf(`
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
			system_disk = {
				name       = "System"
				size_in_gb = 20
			}
		}

		resource "katapult_disk" "data_a" {
			name       = "Data A"
			size_in_gb = 10
		}

		resource "katapult_disk" "data_b" {
			name       = "Data B"
			size_in_gb = 10
		}

		resource "katapult_disk_assignment" "data_a" {
			virtual_machine_id = katapult_virtual_machine.base.id
			disk_id            = katapult_disk.data_a.id
		}

		resource "katapult_disk_assignment" "data_b" {
			virtual_machine_id = katapult_virtual_machine.base.id
			disk_id            = katapult_disk.data_b.id
		}

		data "katapult_virtual_machine_disks" "all" {
			virtual_machine_id = katapult_virtual_machine.base.id
			depends_on = [
				katapult_disk_assignment.data_a,
				katapult_disk_assignment.data_b,
			]
		}`, name, name+"-host")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckKatapultVirtualMachineDestroy(tt),
			testAccCheckKatapultDiskDestroy(tt),
			testAccCheckKatapultIPDestroy(tt),
		),
		Steps: []resource.TestStep{
			{
				Config: legacyConfig,
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
						"package", "rock-3",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_virtual_machine_disks.all", "disks.#", "3",
					),
					captureResourceAttr("katapult_virtual_machine.base", "id", &vmID),
					captureResourceAttr("katapult_virtual_machine.base", "system_disk.id", &bootDiskID),
					captureVMDiskIDByName("data.katapult_virtual_machine_disks.all", "Data A", &dataDiskAID),
					captureVMDiskIDByName("data.katapult_virtual_machine_disks.all", "Data B", &dataDiskBID),
				),
			},
			{
				Config:             importConfig,
				ResourceName:       "katapult_disk.data_a",
				ImportState:        true,
				ImportStatePersist: true,
				ImportStateIdFunc: func(*terraform.State) (string, error) {
					return dataDiskAID, nil
				},
			},
			{
				Config:             importConfig,
				ResourceName:       "katapult_disk.data_b",
				ImportState:        true,
				ImportStatePersist: true,
				ImportStateIdFunc: func(*terraform.State) (string, error) {
					return dataDiskBID, nil
				},
			},
			{
				Config:             importConfig,
				ResourceName:       "katapult_disk_assignment.data_a",
				ImportState:        true,
				ImportStatePersist: true,
				ImportStateIdFunc: func(*terraform.State) (string, error) {
					return assignmentID(vmID, dataDiskAID), nil
				},
			},
			{
				Config:             importConfig,
				ResourceName:       "katapult_disk_assignment.data_b",
				ImportState:        true,
				ImportStatePersist: true,
				ImportStateIdFunc: func(*terraform.State) (string, error) {
					return assignmentID(vmID, dataDiskBID), nil
				},
			},
			{
				Config: firstClassConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPtr("katapult_virtual_machine.base", "id", &vmID),
					resource.TestCheckResourceAttrPtr("katapult_virtual_machine.base", "system_disk.id", &bootDiskID),
					resource.TestCheckResourceAttrPtr("katapult_disk.data_a", "id", &dataDiskAID),
					resource.TestCheckResourceAttrPtr("katapult_disk.data_b", "id", &dataDiskBID),
					resource.TestCheckResourceAttr("katapult_disk_assignment.data_a", "attached", "true"),
					resource.TestCheckResourceAttr("katapult_disk_assignment.data_b", "attached", "true"),
					resource.TestCheckResourceAttr("data.katapult_virtual_machine_disks.all", "disks.#", "3"),
				),
			},
			{
				Config:   firstClassConfig,
				PlanOnly: true,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

func captureVMDiskIDByName(
	dataSourceAddress, name string,
	target *string,
) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		dataSource, ok := state.RootModule().Resources[dataSourceAddress]
		if !ok {
			return fmt.Errorf("resource not found: %s", dataSourceAddress)
		}
		count, err := strconv.Atoi(dataSource.Primary.Attributes["disks.#"])
		if err != nil {
			return fmt.Errorf("reading %s disk count: %w", dataSourceAddress, err)
		}
		for i := range count {
			prefix := fmt.Sprintf("disks.%d.", i)
			if dataSource.Primary.Attributes[prefix+"name"] == name {
				*target = dataSource.Primary.Attributes[prefix+"id"]
				if *target == "" {
					return fmt.Errorf("disk %q has no ID in %s", name, dataSourceAddress)
				}
				return nil
			}
		}
		return fmt.Errorf("disk %q not found in %s", name, dataSourceAddress)
	}
}

func TestAccKatapultVirtualMachine_update(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
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
								`(?i)^%s\..+$`,
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
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectUnknownValue(
							"katapult_virtual_machine.base",
							tfjsonpath.New("state"),
						),
						plancheck.ExpectUnknownValue(
							"katapult_virtual_machine.base",
							tfjsonpath.New("powered_on"),
						),
					},
				},
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
								`(?i)^%s\..+$`,
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
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base", "powered_on", "true",
					),
				),
			},
		},
	})
}

func TestAccKatapultVirtualMachine_update_ips(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
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

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
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
					resource.TestCheckNoResourceAttr(
						"katapult_virtual_machine.base",
						"group_id",
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
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
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

func TestAccKatapultVirtualMachine_disk_assignment(t *testing.T) {
	tt := newTestTools(t)

	diskName := tt.ResourceName("disk")
	assignmentConfig := func(attached bool) string {
		return undent.Stringf(`
			resource "katapult_ip" "web" {}

			resource "katapult_disk" "data" {
			  name       = "%s"
			  size_in_gb = 20
			}

			resource "katapult_virtual_machine" "base" {
			  package       = "rock-3"
			  disk_template = "ubuntu-18-04"
			  disk_template_options = {
			    install_agent = true
			  }
			  ip_address_ids = [katapult_ip.web.id]
			}

			resource "katapult_disk_assignment" "data" {
			  virtual_machine_id = katapult_virtual_machine.base.id
			  disk_id            = katapult_disk.data.id
			  attached           = %t
			}`, diskName, attached,
		)
	}

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckKatapultVirtualMachineDestroy(tt),
			testAccCheckKatapultDiskDestroy(tt),
			testAccCheckKatapultIPDestroy(tt),
		),
		Steps: []resource.TestStep{
			{
				Config: assignmentConfig(true),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVirtualMachineExists(
						tt, "katapult_virtual_machine.base",
					),
					resource.TestCheckResourceAttr(
						"katapult_disk_assignment.data", "attached", "true",
					),
					resource.TestCheckResourceAttr(
						"katapult_disk_assignment.data", "attach_on_boot", "true",
					),
					resource.TestCheckResourceAttr(
						"katapult_disk_assignment.data", "attachment_state", "attached",
					),
				),
			},
			{
				Config: assignmentConfig(false),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectUnknownValue(
							"katapult_disk_assignment.data",
							tfjsonpath.New("attach_on_boot"),
						),
						plancheck.ExpectUnknownValue(
							"katapult_disk_assignment.data",
							tfjsonpath.New("attachment_state"),
						),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"katapult_disk_assignment.data", "attached", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_disk_assignment.data", "attach_on_boot", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_disk_assignment.data", "attachment_state", "detached",
					),
				),
			},
			{
				Config: assignmentConfig(true),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectUnknownValue(
							"katapult_disk_assignment.data",
							tfjsonpath.New("attach_on_boot"),
						),
						plancheck.ExpectUnknownValue(
							"katapult_disk_assignment.data",
							tfjsonpath.New("attachment_state"),
						),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"katapult_disk_assignment.data", "attached", "true",
					),
					resource.TestCheckResourceAttr(
						"katapult_disk_assignment.data", "attach_on_boot", "true",
					),
					resource.TestCheckResourceAttr(
						"katapult_disk_assignment.data", "attachment_state", "attached",
					),
				),
			},
			{
				ResourceName: "katapult_disk_assignment.data",
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["katapult_disk_assignment.data"]
					if rs == nil {
						return "", fmt.Errorf("resource not found")
					}
					return rs.Primary.Attributes["virtual_machine_id"] + "/" + rs.Primary.Attributes["disk_id"], nil
				},
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
			{
				// Remove the VM but keep the disk — disk should survive.
				Config: undent.Stringf(`
					resource "katapult_ip" "web" {}

					resource "katapult_disk" "data" {
					  name       = "%s"
					  size_in_gb = 20
					}`, diskName,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultDiskExists(
						tt, "katapult_disk.data",
					),
				),
			},
		},
	})
}

//nolint:lll // Keep acceptance assertions adjacent to each scenario.
func TestAccKatapultVirtualMachine_system_disk(t *testing.T) {
	tt := newTestTools(t)
	name := tt.ResourceName()
	var vmID, diskID string

	config := func(diskName string, size *int, poweredOn bool, method string) string {
		sizeConfig := ""
		if size != nil {
			sizeConfig = fmt.Sprintf("size_in_gb = %d", *size)
		}
		methodConfig := ""
		if method != "" {
			methodConfig = fmt.Sprintf("resize_method = %q", method)
		}
		return undent.Stringf(`
			resource "katapult_ip" "web" {}

			resource "katapult_virtual_machine" "base" {
			  name          = "%s"
			  package       = "rock-3"
			  disk_template = "ubuntu-18-04"
			  disk_template_options = {
			    install_agent = true
			  }
			  ip_address_ids = [katapult_ip.web.id]
			  powered_on    = %t

			  system_disk = {
			    name = "%s"
			    %s
			    %s
			  }

			  timeouts {
			    update = "45m"
			  }
			}`, name, poweredOn, diskName, sizeConfig, methodConfig)
	}
	size30, size40 := 30, 40

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckKatapultVirtualMachineDestroy(tt),
			testAccCheckKatapultIPDestroy(tt),
		),
		Steps: []resource.TestStep{
			{
				// Supplying only a name keeps package sizing for VM creation and
				// renames the authoritative boot disk after it is discovered.
				Config: config("System Disk", nil, true, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("katapult_virtual_machine.base", "system_disk.name", "System Disk"),
					resource.TestCheckResourceAttr("katapult_virtual_machine.base", "system_disk.size_in_gb", "25"),
					resource.TestCheckResourceAttr("katapult_virtual_machine.base", "system_disk.resize_method", "offline"),
					resource.TestCheckResourceAttr("katapult_virtual_machine.base", "powered_on", "true"),
					captureResourceAttr("katapult_virtual_machine.base", "id", &vmID),
					captureResourceAttr("katapult_virtual_machine.base", "system_disk.id", &diskID),
				),
			},
			{
				// Running system disks require an explicit online resize opt-in.
				Config: config("System Disk", &size30, true, "online"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("katapult_virtual_machine.base", "system_disk.size_in_gb", "30"),
					resource.TestCheckResourceAttr("katapult_virtual_machine.base", "system_disk.resize_method", "online"),
					resource.TestCheckResourceAttr("katapult_virtual_machine.base", "powered_on", "true"),
					resource.TestCheckResourceAttrPtr("katapult_virtual_machine.base", "id", &vmID),
					resource.TestCheckResourceAttrPtr("katapult_virtual_machine.base", "system_disk.id", &diskID),
				),
			},
			{
				// One apply must shut the VM down before performing the offline
				// boot-disk resize; both identities remain stable.
				Config: config("System Disk Resized", &size40, false, "offline"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("katapult_virtual_machine.base", "system_disk.name", "System Disk Resized"),
					resource.TestCheckResourceAttr("katapult_virtual_machine.base", "system_disk.size_in_gb", "40"),
					resource.TestCheckResourceAttr("katapult_virtual_machine.base", "powered_on", "false"),
					resource.TestCheckResourceAttr("katapult_virtual_machine.base", "state", "stopped"),
					resource.TestCheckResourceAttrPtr("katapult_virtual_machine.base", "id", &vmID),
					resource.TestCheckResourceAttrPtr("katapult_virtual_machine.base", "system_disk.id", &diskID),
				),
			},
			{
				// A stopped system disk can be shrunk offline; the task must
				// complete and the API must converge before state is updated.
				Config: config("System Disk Resized", &size30, false, "offline"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("katapult_virtual_machine.base", "system_disk.size_in_gb", "30"),
					resource.TestCheckResourceAttr("katapult_virtual_machine.base", "state", "stopped"),
					resource.TestCheckResourceAttrPtr("katapult_virtual_machine.base", "id", &vmID),
					resource.TestCheckResourceAttrPtr("katapult_virtual_machine.base", "system_disk.id", &diskID),
				),
			},
		},
	})
}

func TestAccKatapultVirtualMachine_update_disk_assignments(t *testing.T) {
	tt := newTestTools(t)

	diskAName := tt.ResourceName("disk-a")
	diskBName := tt.ResourceName("disk-b")

	// depends_on serializes the disk create requests so VCR replay can match
	// otherwise-identical PostOrganizationDisks calls in a deterministic order.
	disksConfig := undent.Stringf(`
		resource "katapult_disk" "a" {
		  name       = "%s"
		  size_in_gb = 20
		}

		resource "katapult_disk" "b" {
		  depends_on = [katapult_disk.a]
		  name       = "%s"
		  size_in_gb = 20
		}
	`, diskAName, diskBName)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckKatapultVirtualMachineDestroy(tt),
			testAccCheckKatapultDiskDestroy(tt),
			testAccCheckKatapultIPDestroy(tt),
		),
		Steps: []resource.TestStep{
			{
				Config: disksConfig + undent.String(`
					resource "katapult_ip" "web" {}

					resource "katapult_virtual_machine" "base" {
					  package       = "rock-3"
					  disk_template = "ubuntu-18-04"
					  disk_template_options = {
					    install_agent = true
					  }
					  ip_address_ids = [katapult_ip.web.id]
					}

					resource "katapult_disk_assignment" "a" {
					  virtual_machine_id = katapult_virtual_machine.base.id
					  disk_id            = katapult_disk.a.id
					}`,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"katapult_disk_assignment.a", "attached", "true",
					),
				),
			},
			{
				Config: disksConfig + undent.String(`
					resource "katapult_ip" "web" {}

					resource "katapult_virtual_machine" "base" {
					  package       = "rock-3"
					  disk_template = "ubuntu-18-04"
					  disk_template_options = {
					    install_agent = true
					  }
					  ip_address_ids = [katapult_ip.web.id]
					}

					resource "katapult_disk_assignment" "a" {
					  virtual_machine_id = katapult_virtual_machine.base.id
					  disk_id            = katapult_disk.a.id
					}

					resource "katapult_disk_assignment" "b" {
					  virtual_machine_id = katapult_virtual_machine.base.id
					  disk_id            = katapult_disk.b.id
					}`,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"katapult_disk_assignment.a", "attached", "true",
					),
					resource.TestCheckResourceAttr(
						"katapult_disk_assignment.b", "attached", "true",
					),
				),
			},
			{
				Config: disksConfig + undent.String(`
					resource "katapult_ip" "web" {}

					resource "katapult_virtual_machine" "base" {
					  package       = "rock-3"
					  disk_template = "ubuntu-18-04"
					  disk_template_options = {
					    install_agent = true
					  }
					  ip_address_ids = [katapult_ip.web.id]
					}

					resource "katapult_disk_assignment" "b" {
					  virtual_machine_id = katapult_virtual_machine.base.id
					  disk_id            = katapult_disk.b.id
					}`,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"katapult_disk_assignment.b", "attached", "true",
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
		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("resource not found: %s", res)
		}

		vmRes, err := tt.Meta.Core.GetVirtualMachineWithResponse(tt.Ctx,
			&core.GetVirtualMachineParams{
				VirtualMachineId: &rs.Primary.ID,
			})
		if err != nil {
			return err
		}

		if vmRes.JSON200 == nil {
			return fmt.Errorf(
				"katapult_virtual_machine %s not found", rs.Primary.ID,
			)
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

			vmRes, err := m.Core.GetVirtualMachineWithResponse(tt.Ctx,
				&core.GetVirtualMachineParams{
					VirtualMachineId: &rs.Primary.ID,
				})
			if err == nil && vmRes.JSON200 != nil {
				return fmt.Errorf(
					"katapult_virtual_machine %s (%s) was not destroyed",
					rs.Primary.ID, *vmRes.JSON200.VirtualMachine.Name,
				)
			}
			if err != nil && !errors.Is(err, core.ErrNotFound) {
				return err
			}

			trashRes, err := m.Core.GetTrashObjectWithResponse(tt.Ctx,
				&core.GetTrashObjectParams{
					TrashObjectObjectId: &rs.Primary.ID,
				})
			if err != nil {
				if trashRes != nil && trashRes.JSON404 != nil {
					continue
				}
				if errors.Is(err, core.ErrNotFound) {
					continue
				}

				return fmt.Errorf(
					"error looking up trashed katapult_virtual_machine %s: %w",
					rs.Primary.ID,
					err,
				)
			}
			if trashRes.JSON200 != nil {
				return fmt.Errorf(
					"katapult_virtual_machine %s was deleted "+
						"but not purged from trash",
					rs.Primary.ID,
				)
			}
		}

		return nil
	}
}
