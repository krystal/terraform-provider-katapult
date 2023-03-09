package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceSecurityGroup_minimal(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultSecurityGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"
					}

					data "katapult_security_group" "my_sg" {
						id = katapult_security_group.my_sg.id
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "data.katapult_security_group.my_sg",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group.my_sg", "name",
						"katapult_security_group.my_sg", "name",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group.my_sg", "associations",
						"katapult_security_group.my_sg", "associations",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group.my_sg",
						"allow_all_inbound",
						"katapult_security_group.my_sg",
						"allow_all_inbound",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group.my_sg",
						"allow_all_outbound",
						"katapult_security_group.my_sg",
						"allow_all_outbound",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group.my_sg",
						"inbound_rules",
						"katapult_security_group.my_sg",
						"inbound_rule",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group.my_sg",
						"outbound_rules",
						"katapult_security_group.my_sg",
						"outbound_rule",
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceSecurityGroup_include_rules(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultSecurityGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_virtual_machine_group" "web" {
						name = "%s"
					}

					resource "katapult_security_group" "my_sg" {
						name = "%s"
						associations = [
							katapult_virtual_machine_group.web.id,
						]
						allow_all_inbound = false
						allow_all_outbound = true

						inbound_rule {
							protocol = "tcp"
							ports = "22"
							targets = ["10.0.0.0/8"]
							notes = "SSH"
						}
						inbound_rule {
							protocol = "tcp"
							ports = "80"
							targets = ["all:ipv4", "all:ipv6"]
							notes = "HTTP"
						}
					}

					data "katapult_security_group" "my_sg" {
						id = katapult_security_group.my_sg.id
					}`,
					name, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "data.katapult_security_group.my_sg",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group.my_sg", "name",
						"katapult_security_group.my_sg", "name",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group.my_sg", "associations",
						"katapult_security_group.my_sg", "associations",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group.my_sg",
						"allow_all_inbound",
						"katapult_security_group.my_sg",
						"allow_all_inbound",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group.my_sg",
						"allow_all_outbound",
						"katapult_security_group.my_sg",
						"allow_all_outbound",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group.my_sg",
						"inbound_rules",
						"katapult_security_group.my_sg",
						"inbound_rule",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group.my_sg",
						"outbound_rules",
						"katapult_security_group.my_sg",
						"outbound_rule",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_virtual_machine_group" "web" {
						name = "%s"
					}

					resource "katapult_virtual_machine_group" "db" {
						name = "%s"
					}

					resource "katapult_security_group" "my_sg" {
						name = "%s"
						associations = [
							katapult_virtual_machine_group.web.id,
						]
						allow_all_inbound = false
						allow_all_outbound = false

						inbound_rule {
							protocol = "tcp"
							ports = "22"
							targets = ["10.0.0.0/8"]
							notes = "SSH"
						}
						inbound_rule {
							protocol = "tcp"
							ports = "80"
							targets = ["all:ipv4", "all:ipv6"]
							notes = "HTTP"
						}

						outbound_rule {
							protocol = "tcp"
							ports = "3306"
							targets = [katapult_virtual_machine_group.db.id]
							notes = "MySQL"
						}
						outbound_rule {
							protocol = "tcp"
							ports = "80,443"
							targets = ["all:ipv4", "all:ipv6"]
							notes = "HTTP & HTTPS"
						}
					}

					data "katapult_security_group" "my_sg" {
						id = katapult_security_group.my_sg.id
					}`,
					name, name, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "data.katapult_security_group.my_sg",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group.my_sg", "name",
						"katapult_security_group.my_sg", "name",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group.my_sg", "associations",
						"katapult_security_group.my_sg", "associations",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group.my_sg",
						"allow_all_inbound",
						"katapult_security_group.my_sg",
						"allow_all_inbound",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group.my_sg",
						"allow_all_outbound",
						"katapult_security_group.my_sg",
						"allow_all_outbound",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group.my_sg",
						"inbound_rules",
						"katapult_security_group.my_sg",
						"inbound_rule",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group.my_sg",
						"outbound_rules",
						"katapult_security_group.my_sg",
						"outbound_rule",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_virtual_machine_group" "web" {
						name = "%s"
					}

					resource "katapult_virtual_machine_group" "db" {
						name = "%s"
					}

					resource "katapult_security_group" "my_sg" {
						name = "%s"
						associations = [
							katapult_virtual_machine_group.web.id,
						]
						allow_all_inbound = true
						allow_all_outbound = false

						outbound_rule {
							protocol = "tcp"
							ports = "3306"
							targets = [katapult_virtual_machine_group.db.id]
							notes = "MySQL"
						}
						outbound_rule {
							protocol = "tcp"
							ports = "80,443"
							targets = ["all:ipv4", "all:ipv6"]
							notes = "HTTP & HTTPS"
						}
					}

					data "katapult_security_group" "my_sg" {
						id = katapult_security_group.my_sg.id
					}`,
					name, name, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "data.katapult_security_group.my_sg",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group.my_sg", "name",
						"katapult_security_group.my_sg", "name",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group.my_sg", "associations",
						"katapult_security_group.my_sg", "associations",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group.my_sg",
						"allow_all_inbound",
						"katapult_security_group.my_sg",
						"allow_all_inbound",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group.my_sg",
						"allow_all_outbound",
						"katapult_security_group.my_sg",
						"allow_all_outbound",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group.my_sg",
						"inbound_rules",
						"katapult_security_group.my_sg",
						"inbound_rule",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group.my_sg",
						"outbound_rules",
						"katapult_security_group.my_sg",
						"outbound_rule",
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceSecurityGroup_no_include_rules(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultSecurityGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_virtual_machine_group" "web" {
						name = "%s"
					}

					resource "katapult_security_group" "my_sg" {
						name = "%s"
						associations = [
							katapult_virtual_machine_group.web.id,
						]
						allow_all_inbound = false
						allow_all_outbound = true

						inbound_rule {
							protocol = "tcp"
							ports = "22"
							targets = ["10.0.0.0/8"]
							notes = "SSH"
						}
						inbound_rule {
							protocol = "tcp"
							ports = "80"
							targets = ["all:ipv4", "all:ipv6"]
							notes = "HTTP"
						}
						inbound_rule {
							protocol = "TCP"
							ports = "443"
							targets = ["all:ipv4", "all:ipv6"]
							notes = "HTTPS"
						}
						inbound_rule {
							protocol = "icmp"
							targets = ["219.185.152.0/24"]
							notes = "ping"
						}
						inbound_rule {
							protocol = "udp"
							ports = "443"
							targets = ["all:ipv4", "all:ipv6"]
							notes = "QUIC"
						}
					}

					data "katapult_security_group" "my_sg" {
						id = katapult_security_group.my_sg.id
						include_rules = false
					}`,
					name, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "data.katapult_security_group.my_sg",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group.my_sg", "name",
						"katapult_security_group.my_sg", "name",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group.my_sg", "associations",
						"katapult_security_group.my_sg", "associations",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group.my_sg",
						"allow_all_inbound",
						"katapult_security_group.my_sg",
						"allow_all_inbound",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group.my_sg",
						"allow_all_outbound",
						"katapult_security_group.my_sg",
						"allow_all_outbound",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_security_group.my_sg",
						"inbound_rule.#", "0",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_security_group.my_sg",
						"outbound_rule.#", "0",
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceSecurityGroup_not_found(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultSecurityGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					data "katapult_security_group" "my_sg" {
						id = "sg_thisdoesnotexist"
					}`,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						"security_group_not_found: " +
							"No security group was found matching any " +
							"of the criteria provided in the arguments",
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceSecurityGroup_blank(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultSecurityGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					data "katapult_security_group" "my_sg" {

					}`,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						"The argument \"id\" is required, but no definition " +
							"was found.",
					),
				),
			},
		},
	})
}
