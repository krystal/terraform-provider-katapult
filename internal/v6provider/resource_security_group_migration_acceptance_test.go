package v6provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/jimeh/undent"
)

//nolint:lll // The complete handover and representation matrix is one lifecycle.
func TestAccKatapultSecurityGroup_migrate_v5_blocks_and_round_trip(t *testing.T) {
	tt := newTestTools(t)
	name := tt.ResourceName("sg-migrate")
	var groupID, sshID, firstICMPID, secondICMPID, dnsID, webID string

	blocks := securityGroupMigrationConfig(name, securityGroupMigrationBlockRules(), false)
	attributes := securityGroupMigrationConfig(name, securityGroupMigrationAttributeRules(), false)
	attributesMaterial := securityGroupMigrationConfig(name, securityGroupMigrationAttributeRulesWithPorts("2222", "53"), false)
	mixedInboundBlocks := securityGroupMigrationConfig(name, securityGroupMigrationMixedRules(true, "2222", "53"), false)
	mixedOutboundBlocks := securityGroupMigrationConfig(name, securityGroupMigrationMixedRules(false, "2222", "53"), false)
	blocksAfterAttributes := securityGroupMigrationConfig(name, securityGroupMigrationBlockRulesWithPorts("2222", "53"), false)
	blocksMaterial := securityGroupMigrationConfig(name, securityGroupMigrationBlockRulesWithPorts("2222", "5353"), false)
	withDataSources := securityGroupMigrationConfig(name, securityGroupMigrationBlockRulesWithPorts("2222", "5353"), true)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		Steps: []resource.TestStep{
			{
				ProtoV5ProviderFactories: tt.LegacySecurityGroupFactories,
				Config:                   blocks,
				Check: resource.ComposeAggregateTestCheckFunc(
					captureResourceAttr("katapult_security_group.main", "id", &groupID),
					captureResourceAttr("katapult_security_group.main", "inbound_rule.0.id", &sshID),
					captureResourceAttr("katapult_security_group.main", "inbound_rule.1.id", &firstICMPID),
					captureResourceAttr("katapult_security_group.main", "inbound_rule.2.id", &secondICMPID),
					captureResourceAttr("katapult_security_group.main", "outbound_rule.0.id", &dnsID),
					captureResourceAttr("katapult_security_group.main", "outbound_rule.1.id", &webID),
				),
			},
			{
				ProtoV6ProviderFactories: tt.ProviderFactories,
				Config:                   blocks,
				PlanOnly:                 true,
				ConfigPlanChecks:         emptyPostRefreshPlanChecks(),
			},
			{
				ProtoV6ProviderFactories: tt.ProviderFactories,
				Config:                   attributes,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPtr("katapult_security_group.main", "id", &groupID),
					resource.TestCheckResourceAttrPtr("katapult_security_group.main", "inbound_rules.0.id", &firstICMPID),
					resource.TestCheckResourceAttrPtr("katapult_security_group.main", "inbound_rules.1.id", &sshID),
					resource.TestCheckResourceAttrPtr("katapult_security_group.main", "inbound_rules.2.id", &secondICMPID),
					resource.TestCheckResourceAttrPtr("katapult_security_group.main", "outbound_rules.0.id", &webID),
					resource.TestCheckResourceAttrPtr("katapult_security_group.main", "outbound_rules.1.id", &dnsID),
				),
			},
			{
				ProtoV6ProviderFactories: tt.ProviderFactories,
				Config:                   attributes,
				PlanOnly:                 true,
				ConfigPlanChecks:         emptyPostRefreshPlanChecks(),
			},
			{
				ProtoV6ProviderFactories: tt.ProviderFactories,
				Config:                   attributesMaterial,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPtr("katapult_security_group.main", "id", &groupID),
					resource.TestCheckResourceAttrPtr("katapult_security_group.main", "inbound_rules.0.id", &firstICMPID),
					resource.TestCheckResourceAttrPtr("katapult_security_group.main", "inbound_rules.1.id", &sshID),
					resource.TestCheckResourceAttr("katapult_security_group.main", "inbound_rules.1.ports", "2222"),
					resource.TestCheckResourceAttrPtr("katapult_security_group.main", "inbound_rules.2.id", &secondICMPID),
					resource.TestCheckResourceAttrPtr("katapult_security_group.main", "outbound_rules.0.id", &webID),
					resource.TestCheckResourceAttrPtr("katapult_security_group.main", "outbound_rules.1.id", &dnsID),
				),
			},
			{
				ProtoV6ProviderFactories: tt.ProviderFactories,
				Config:                   attributesMaterial,
				PlanOnly:                 true,
				ConfigPlanChecks:         emptyPostRefreshPlanChecks(),
			},
			{ProtoV6ProviderFactories: tt.ProviderFactories, Config: mixedInboundBlocks},
			{ProtoV6ProviderFactories: tt.ProviderFactories, Config: mixedOutboundBlocks},
			{
				ProtoV6ProviderFactories: tt.ProviderFactories,
				Config:                   blocksAfterAttributes,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPtr("katapult_security_group.main", "inbound_rule.0.id", &sshID),
					resource.TestCheckResourceAttrPtr("katapult_security_group.main", "inbound_rule.1.id", &firstICMPID),
					resource.TestCheckResourceAttrPtr("katapult_security_group.main", "inbound_rule.2.id", &secondICMPID),
					resource.TestCheckResourceAttrPtr("katapult_security_group.main", "outbound_rule.0.id", &dnsID),
					resource.TestCheckResourceAttrPtr("katapult_security_group.main", "outbound_rule.1.id", &webID),
				),
			},
			{
				ProtoV6ProviderFactories: tt.ProviderFactories,
				Config:                   blocksAfterAttributes,
				PlanOnly:                 true,
				ConfigPlanChecks:         emptyPostRefreshPlanChecks(),
			},
			{
				ProtoV6ProviderFactories: tt.ProviderFactories,
				Config:                   blocksMaterial,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPtr("katapult_security_group.main", "id", &groupID),
					resource.TestCheckResourceAttrPtr("katapult_security_group.main", "inbound_rule.0.id", &sshID),
					resource.TestCheckResourceAttrPtr("katapult_security_group.main", "inbound_rule.1.id", &firstICMPID),
					resource.TestCheckResourceAttrPtr("katapult_security_group.main", "inbound_rule.2.id", &secondICMPID),
					resource.TestCheckResourceAttrPtr("katapult_security_group.main", "outbound_rule.0.id", &dnsID),
					resource.TestCheckResourceAttr("katapult_security_group.main", "outbound_rule.0.ports", "5353"),
					resource.TestCheckResourceAttrPtr("katapult_security_group.main", "outbound_rule.1.id", &webID),
				),
			},
			{
				ProtoV6ProviderFactories: tt.ProviderFactories,
				Config:                   blocksMaterial,
				PlanOnly:                 true,
				ConfigPlanChecks:         emptyPostRefreshPlanChecks(),
			},
			{
				ProtoV6ProviderFactories: tt.ProviderFactories,
				Config:                   withDataSources,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("katapult_security_group.minimal", "associations.#", "0"),
					resource.TestCheckResourceAttr("katapult_security_group.allow", "allow_all_inbound", "true"),
					resource.TestCheckResourceAttr("katapult_security_group.allow", "allow_all_outbound", "true"),
					resource.TestCheckResourceAttr("katapult_security_group_rule.standalone", "protocol", "TCP"),
					resource.TestCheckResourceAttr("katapult_security_group_rule.standalone", "ports", ""),
					resource.TestCheckResourceAttr("katapult_security_group_rule.standalone", "notes", ""),
					resource.TestCheckResourceAttr("data.katapult_security_group.group", "name", name+"-main"),
					resource.TestCheckResourceAttr("data.katapult_security_group_rule.rule", "direction", "inbound"),
					resource.TestCheckResourceAttrSet("data.katapult_security_group_rules.rules", "id"),
					resource.TestCheckResourceAttrSet("data.katapult_security_groups.groups", "id"),
				),
			},
			{
				ProtoV6ProviderFactories: tt.ProviderFactories,
				ResourceName:             "katapult_security_group.main",
				ImportState:              true,
				ImportStateVerify:        true,
				ImportStateVerifyIgnore:  securityGroupRuleRepresentationAttributes,
			},
			{
				ProtoV6ProviderFactories: tt.ProviderFactories,
				Config:                   withDataSources,
				PlanOnly:                 true,
				ConfigPlanChecks:         emptyPostRefreshPlanChecks(),
			},
			{
				ProtoV6ProviderFactories: tt.ProviderFactories,
				ResourceName:             "katapult_security_group_rule.standalone",
				ImportState:              true,
				ImportStateVerify:        true,
				ImportStateVerifyIgnore:  []string{"protocol"},
			},
			{
				ProtoV6ProviderFactories: tt.ProviderFactories,
				Config:                   withDataSources,
				PlanOnly:                 true,
				ConfigPlanChecks:         emptyPostRefreshPlanChecks(),
			},
		},
	})
}

func emptyPostRefreshPlanChecks() resource.ConfigPlanChecks {
	return resource.ConfigPlanChecks{
		PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
	}
}

func securityGroupMigrationConfig(name, rules string, includeDataSources bool) string {
	dataSources := ""
	if includeDataSources {
		dataSources = `
		data "katapult_security_group" "group" {
			id = katapult_security_group.main.id
		}

		data "katapult_security_group_rule" "rule" {
			id = katapult_security_group_rule.standalone.id
		}

		data "katapult_security_group_rules" "rules" {
			security_group_id = katapult_security_group.external.id
		}

		data "katapult_security_groups" "groups" {
			include_rules = true
		}
		`
	}

	return undent.Stringf(`
		resource "katapult_legacy_virtual_machine_group" "association" {
			name = "%s-association"
		}

		resource "katapult_security_group" "minimal" {
			name       = "%s-minimal"
			depends_on = [katapult_legacy_virtual_machine_group.association]
		}

		resource "katapult_security_group" "allow" {
			name               = "%s-allow"
			associations       = [katapult_legacy_virtual_machine_group.association.id]
			allow_all_inbound  = true
			allow_all_outbound = true
			depends_on         = [katapult_security_group.minimal]
		}

		resource "katapult_security_group" "main" {
			name         = "%s-main"
			associations = [katapult_legacy_virtual_machine_group.association.id]
			%s
			depends_on = [katapult_security_group.allow]
		}

		resource "katapult_security_group" "external" {
			name           = "%s-external"
			external_rules = true
			depends_on     = [katapult_security_group.main]
		}

		resource "katapult_security_group_rule" "standalone" {
			security_group_id = katapult_security_group.external.id
			direction         = "inbound"
			protocol          = "TCP"
			targets           = ["all:ipv4"]
		}
		%s
	`, name, name, name, name, rules, name, dataSources)
}

func securityGroupMigrationBlockRules() string {
	return securityGroupMigrationBlockRulesWithPorts("22", "53")
}

func securityGroupMigrationBlockRulesWithPorts(sshPorts, dnsPorts string) string {
	return undent.Stringf(`
		inbound_rule {
			protocol = "tcp"
			ports    = "%s"
			targets  = ["all:ipv4", "all:ipv6"]
			notes    = "SSH"
		}
		inbound_rule {
			protocol = "icmp"
			targets  = ["all:ipv4"]
		}
		inbound_rule {
			protocol = "ICMP"
			targets  = ["all:ipv4"]
		}
		outbound_rule {
			protocol = "udp"
			ports    = "%s"
			targets  = ["all:ipv4"]
		}
		outbound_rule {
			protocol = "tcp"
			targets  = ["all:ipv6"]
			notes    = "Web"
		}
	`, sshPorts, dnsPorts)
}

func securityGroupMigrationAttributeRules() string {
	return securityGroupMigrationAttributeRulesWithPorts("22", "53")
}

func securityGroupMigrationAttributeRulesWithPorts(sshPorts, dnsPorts string) string {
	return undent.Stringf(`
		inbound_rules = [for rule in [
			{ protocol = "icmp", targets = ["all:ipv4"] },
			{ protocol = "TCP", ports = "%s", targets = ["all:ipv6", "all:ipv4"], notes = "SSH" },
			{ protocol = "ICMP", targets = ["all:ipv4"] },
		] : rule]
		outbound_rules = [
			{ protocol = "tcp", targets = ["all:ipv6"], notes = "Web" },
			{ protocol = "UDP", ports = "%s", targets = ["all:ipv4"] },
		]
	`, sshPorts, dnsPorts)
}

func securityGroupMigrationMixedRules(inboundBlocks bool, sshPorts, dnsPorts string) string {
	if inboundBlocks {
		return undent.Stringf(`
			inbound_rule {
				protocol = "tcp"
				ports    = "%s"
				targets  = ["all:ipv4", "all:ipv6"]
				notes    = "SSH"
			}
			inbound_rule {
				protocol = "icmp"
				targets  = ["all:ipv4"]
			}
			inbound_rule {
				protocol = "ICMP"
				targets  = ["all:ipv4"]
			}
			outbound_rules = [
				{ protocol = "tcp", targets = ["all:ipv6"], notes = "Web" },
				{ protocol = "UDP", ports = "%s", targets = ["all:ipv4"] },
			]
		`, sshPorts, dnsPorts)
	}
	return undent.Stringf(`
		inbound_rules = [
			{ protocol = "icmp", targets = ["all:ipv4"] },
			{ protocol = "TCP", ports = "%s", targets = ["all:ipv6", "all:ipv4"], notes = "SSH" },
			{ protocol = "ICMP", targets = ["all:ipv4"] },
		]
		outbound_rule {
			protocol = "udp"
			ports    = "%s"
			targets  = ["all:ipv4"]
		}
		outbound_rule {
			protocol = "tcp"
			targets  = ["all:ipv6"]
			notes    = "Web"
		}
	`, sshPorts, dnsPorts)
}
