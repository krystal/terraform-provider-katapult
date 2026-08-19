package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultSecurityGroupRule_external_rules_coexistence(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()
	config := undent.Stringf(`
		resource "katapult_security_group" "my_sg" {
			name           = "%s"
			external_rules = true
		}

		resource "katapult_security_group_rule" "ssh" {
			security_group_id = katapult_security_group.my_sg.id
			direction         = "inbound"
			protocol          = "tcp"
			ports             = "22"
			targets           = ["all:ipv4"]
			notes             = "SSH"
		}

		resource "katapult_security_group_rule" "dns" {
			security_group_id = katapult_security_group.my_sg.id
			direction         = "outbound"
			protocol          = "udp"
			ports             = "53"
			targets           = ["all:ipv4", "all:ipv6"]
			notes             = "DNS"

			# Keep VCR response assignment deterministic for the two rule creates.
			depends_on = [katapult_security_group_rule.ssh]
		}`,
		name,
	)

	checks := resource.ComposeAggregateTestCheckFunc(
		testAccCheckKatapultSecurityGroupExists(
			tt, "katapult_security_group.my_sg",
		),
		testAccCheckKatapultSecurityGroupRuleExists(
			tt, "katapult_security_group_rule.ssh",
		),
		testAccCheckKatapultSecurityGroupRuleExists(
			tt, "katapult_security_group_rule.dns",
		),
		resource.TestCheckResourceAttr(
			"katapult_security_group.my_sg", "inbound_rule.#", "0",
		),
		resource.TestCheckResourceAttr(
			"katapult_security_group.my_sg", "outbound_rule.#", "0",
		),
	)

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckKatapultSecurityGroupRuleDestroy(tt),
			testAccCheckKatapultSecurityGroupDestroy(tt),
		),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  checks,
			},
			{
				RefreshState: true,
				Check:        checks,
			},
		},
	})
}

func TestAccKatapultSecurityGroupRule_empty_optional_values(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()
	omittedConfig := undent.Stringf(`
		resource "katapult_security_group" "my_sg" {
			name           = "%s"
			external_rules = true
		}

		resource "katapult_security_group_rule" "my_rule" {
			security_group_id = katapult_security_group.my_sg.id
			direction         = "inbound"
			protocol          = "icmp"
			targets           = []
		}`,
		name,
	)
	explicitConfig := undent.Stringf(`
		resource "katapult_security_group" "my_sg" {
			name           = "%s"
			external_rules = true
		}

		resource "katapult_security_group_rule" "my_rule" {
			security_group_id = katapult_security_group.my_sg.id
			direction         = "inbound"
			protocol          = "icmp"
			ports             = ""
			targets           = []
			notes             = ""
		}`,
		name,
	)

	checks := resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckResourceAttr(
			"katapult_security_group.my_sg", "associations.#", "0",
		),
		resource.TestCheckResourceAttr(
			"katapult_security_group.my_sg", "inbound_rule.#", "0",
		),
		resource.TestCheckResourceAttr(
			"katapult_security_group.my_sg", "outbound_rule.#", "0",
		),
		resource.TestCheckResourceAttr(
			"katapult_security_group_rule.my_rule", "ports", "",
		),
		resource.TestCheckResourceAttr(
			"katapult_security_group_rule.my_rule", "notes", "",
		),
	)

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckKatapultSecurityGroupRuleDestroy(tt),
			testAccCheckKatapultSecurityGroupDestroy(tt),
		),
		Steps: []resource.TestStep{
			{
				Config: omittedConfig,
				Check:  checks,
			},
			{
				Config: explicitConfig,
				Check:  checks,
			},
			{
				Config: omittedConfig,
				Check:  checks,
			},
		},
	})
}
