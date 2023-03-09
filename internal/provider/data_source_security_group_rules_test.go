package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceSecurityGroupRules_no_rules(t *testing.T) {
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

					data "katapult_security_group_rules" "my_rules" {
						security_group_id = katapult_security_group.my_sg.id
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "data.katapult_security_group_rules.my_rules",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group_rules.my_rules",
						"inbound_rules",
						"katapult_security_group.my_sg",
						"inbound_rule",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group_rules.my_rules",
						"outbound_rules",
						"katapult_security_group.my_sg",
						"outbound_rule",
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceSecurityGroupRules_rules(t *testing.T) {
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

					data "katapult_security_group_rules" "my_rules" {
						security_group_id = katapult_security_group.my_sg.id
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "data.katapult_security_group_rules.my_rules",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group_rules.my_rules",
						"inbound_rules",
						"katapult_security_group.my_sg",
						"inbound_rule",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group_rules.my_rules",
						"outbound_rules",
						"katapult_security_group.my_sg",
						"outbound_rule",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_virtual_machine_group" "db" {
						name = "%s"
					}

					resource "katapult_security_group" "my_sg" {
						name = "%s"
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

					data "katapult_security_group_rules" "my_rules" {
						security_group_id = katapult_security_group.my_sg.id
					}`,
					name, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "data.katapult_security_group_rules.my_rules",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group_rules.my_rules",
						"inbound_rules",
						"katapult_security_group.my_sg",
						"inbound_rule",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group_rules.my_rules",
						"outbound_rules",
						"katapult_security_group.my_sg",
						"outbound_rule",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_virtual_machine_group" "db" {
						name = "%s"
					}

					resource "katapult_security_group" "my_sg" {
						name = "%s"
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

					data "katapult_security_group_rules" "my_rules" {
						security_group_id = katapult_security_group.my_sg.id
					}`,
					name, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "data.katapult_security_group_rules.my_rules",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group_rules.my_rules",
						"inbound_rules",
						"katapult_security_group.my_sg",
						"inbound_rule",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group_rules.my_rules",
						"outbound_rules",
						"katapult_security_group.my_sg",
						"outbound_rule",
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceSecurityGroupRules_not_found(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultSecurityGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					data "katapult_security_group_rules" "my_rules" {
						security_group_id = "sg_thisdoesnotexist"
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

func TestAccKatapultDataSourceSecurityGroupRules_blank(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultSecurityGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					data "katapult_security_group_rules" "my_rules" {

					}`,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						"The argument \"security_group_id\" is required, " +
							"but no definition was found.",
					),
				),
			},
		},
	})
}
