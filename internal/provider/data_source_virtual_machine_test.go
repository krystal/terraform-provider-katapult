package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceVirtualMachine_by_id(t *testing.T) {
	tt := NewTestTools(t)
	defer tt.Cleanup()

	name := tt.ResourceName("data-source-by-id")

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
					}

					data "katapult_virtual_machine" "src" {
						id = katapult_virtual_machine.base.id
					}`,
					name, name+"-host",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVirtualMachineExists(
						tt, "data.katapult_virtual_machine.src",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_virtual_machine.src",
						"name", name,
					),
					resource.TestCheckResourceAttr(
						"data.katapult_virtual_machine.src",
						"hostname", name+"-host",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_virtual_machine.src",
						"description", "A web server.",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_virtual_machine.src",
						"package", "rock-3",
					),
					resource.TestCheckTypeSetElemAttr(
						"data.katapult_virtual_machine.src", "tags.*", "web",
					),
					resource.TestCheckTypeSetElemAttr(
						"data.katapult_virtual_machine.src",
						"tags.*", "public",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"data.katapult_virtual_machine.src", "ip_address_ids.*",
						"katapult_ip.web", "id",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"data.katapult_virtual_machine.src", "ip_addresses.*",
						"katapult_ip.web", "address",
					),
					// TODO: populate and check disk_template and options when
					// supported by the API.
					resource.TestCheckNoResourceAttr(
						"data.katapult_virtual_machine.src",
						"disk_template",
					),
					resource.TestCheckNoResourceAttr(
						"data.katapult_virtual_machine.src",
						"disk_template_options.%",
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceVirtualMachine_by_fqdn(t *testing.T) {
	tt := NewTestTools(t)
	defer tt.Cleanup()

	name := tt.ResourceName("data-source-by-fqdn")

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
					}

					data "katapult_virtual_machine" "src" {
						fqdn = katapult_virtual_machine.base.fqdn
					}`,
					name, name+"-host",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVirtualMachineExists(
						tt, "data.katapult_virtual_machine.src",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_virtual_machine.src",
						"name", name,
					),
					resource.TestCheckResourceAttr(
						"data.katapult_virtual_machine.src",
						"hostname", name+"-host",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_virtual_machine.src",
						"description", "A web server.",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_virtual_machine.src",
						"package", "rock-3",
					),
					resource.TestCheckTypeSetElemAttr(
						"data.katapult_virtual_machine.src", "tags.*", "web",
					),
					resource.TestCheckTypeSetElemAttr(
						"data.katapult_virtual_machine.src",
						"tags.*", "public",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"data.katapult_virtual_machine.src", "ip_address_ids.*",
						"katapult_ip.web", "id",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"data.katapult_virtual_machine.src", "ip_addresses.*",
						"katapult_ip.web", "address",
					),
					// TODO: populate and check disk_template and options when
					// supported by the API.
					resource.TestCheckNoResourceAttr(
						"data.katapult_virtual_machine.src",
						"disk_template",
					),
					resource.TestCheckNoResourceAttr(
						"data.katapult_virtual_machine.src",
						"disk_template_options.%",
					),
				),
			},
		},
	})
}
