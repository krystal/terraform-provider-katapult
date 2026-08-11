package v6provider

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/jimeh/undent"
)

func TestAccKatapultVirtualMachine_migrate_v5_state(t *testing.T) {
	tt := newTestTools(t)

	name := strings.ToLower(tt.ResourceName())
	hostname := name + "-host"
	var vmID string

	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckKatapultVirtualMachineDestroy(tt),
			testAccCheckKatapultIPDestroy(tt),
		),
		Steps: []resource.TestStep{
			{
				ProtoV5ProviderFactories: tt.LegacyVMFactories,
				Config: virtualMachineV5HandoverConfig(
					name,
					hostname,
					"",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVirtualMachineExists(
						tt,
						"katapult_virtual_machine.base",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"name",
						name,
					),
					captureResourceAttr(
						"katapult_virtual_machine.base",
						"id",
						&vmID,
					),
				),
			},
			{
				ProtoV6ProviderFactories: tt.ProviderFactories,
				Config: virtualMachineV5HandoverConfig(
					name,
					hostname,
					"",
				),
				PlanOnly: true,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				ProtoV6ProviderFactories: tt.ProviderFactories,
				Config: virtualMachineV5HandoverConfig(
					name+"-renamed",
					hostname,
					"",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVirtualMachineExists(
						tt,
						"katapult_virtual_machine.base",
					),
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"name",
						name+"-renamed",
					),
					resource.TestCheckResourceAttrPtr(
						"katapult_virtual_machine.base",
						"id",
						&vmID,
					),
				),
			},
			{
				ProtoV6ProviderFactories: tt.ProviderFactories,
				Config: virtualMachineV5HandoverConfig(
					name+"-renamed",
					hostname,
					"Temporary description",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"katapult_virtual_machine.base",
						"description",
						"Temporary description",
					),
					resource.TestCheckResourceAttrPtr(
						"katapult_virtual_machine.base",
						"id",
						&vmID,
					),
				),
			},
			{
				ProtoV6ProviderFactories: tt.ProviderFactories,
				Config: virtualMachineV5HandoverConfig(
					name+"-renamed",
					hostname,
					"",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(
						"katapult_virtual_machine.base",
						"description",
					),
					resource.TestCheckResourceAttrPtr(
						"katapult_virtual_machine.base",
						"id",
						&vmID,
					),
				),
			},
			{
				ProtoV6ProviderFactories: tt.ProviderFactories,
				ResourceName:             "katapult_virtual_machine.base",
				ImportState:              true,
				ImportStateVerify:        true,
				ImportStateVerifyIgnore: []string{
					"disk_template",
					"disk_template_options",
					"timeouts",
				},
			},
		},
	})
}

func virtualMachineV5HandoverConfig(
	name string,
	hostname string,
	description string,
) string {
	descriptionConfig := ""
	if description != "" {
		descriptionConfig = fmt.Sprintf("description = %q", description)
	}

	return undent.Stringf(`
		resource "katapult_legacy_ip" "web" {}

		resource "katapult_virtual_machine" "base" {
			name          = "%s"
			hostname      = "%s"
			%s
			package       = "rock-3"
			disk_template = "ubuntu-18-04"
			disk_template_options = {
				install_agent = true
			}
			ip_address_ids = [katapult_legacy_ip.web.id]

			timeouts {
				create = "20m"
				update = "10m"
				delete = "10m"
			}
		}`,
		name,
		hostname,
		descriptionConfig,
	)
}
