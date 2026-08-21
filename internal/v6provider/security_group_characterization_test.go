package v6provider

import (
	"errors"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/next/core"
)

func TestAccKatapultSecurityGroup_external_rules_enable(t *testing.T) {
	tt := newSecurityGroupCharacterizationTestTools(t)

	name := tt.ResourceName()
	var inboundRuleID string
	var outboundRuleID string

	managedConfig := undent.Stringf(`
		resource "katapult_security_group" "my_sg" {
			name = "%s"

			inbound_rule {
				protocol = "tcp"
				ports    = "22"
				targets  = ["all:ipv4"]
				notes    = "SSH"
			}

			outbound_rule {
				protocol = "udp"
				ports    = "53"
				targets  = ["all:ipv4", "all:ipv6"]
				notes    = "DNS"
			}
		}`,
		name,
	)
	externalConfig := undent.Stringf(`
		resource "katapult_security_group" "my_sg" {
			name           = "%s"
			external_rules = true
		}`,
		name,
	)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultSecurityGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: managedConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "katapult_security_group.my_sg",
					),
					testAccCaptureSecurityGroupAttr(
						"katapult_security_group.my_sg",
						"inbound_rule.0.id",
						&inboundRuleID,
					),
					testAccCaptureSecurityGroupAttr(
						"katapult_security_group.my_sg",
						"outbound_rule.0.id",
						&outboundRuleID,
					),
				),
			},
			{
				Config: externalConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "katapult_security_group.my_sg",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg", "external_rules", "true",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg", "inbound_rule.#", "0",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg", "outbound_rule.#", "0",
					),
					testAccCheckKatapultSecurityGroupRuleIDAbsent(
						tt, &inboundRuleID,
					),
					testAccCheckKatapultSecurityGroupRuleIDAbsent(
						tt, &outboundRuleID,
					),
				),
			},
			{
				Config:   externalConfig,
				PlanOnly: true,
			},
		},
	})
}

func TestAccKatapultSecurityGroup_external_rules_disable(t *testing.T) {
	tt := newSecurityGroupCharacterizationTestTools(t)

	name := tt.ResourceName()
	var inboundRuleID string
	var outboundRuleID string

	externalConfig := undent.Stringf(`
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
			notes             = "Externally managed SSH"
		}

		resource "katapult_security_group_rule" "dns" {
			security_group_id = katapult_security_group.my_sg.id
			direction         = "outbound"
			protocol          = "udp"
			ports             = "53"
			targets           = ["all:ipv4", "all:ipv6"]
			notes             = "Externally managed DNS"

			depends_on = [katapult_security_group_rule.ssh]
		}`,
		name,
	)
	adoptConfig := undent.Stringf(`
		resource "katapult_security_group" "my_sg" {
			name = "%s"
		}

		resource "katapult_security_group_rule" "ssh" {
			security_group_id = katapult_security_group.my_sg.id
			direction         = "inbound"
			protocol          = "tcp"
			ports             = "22"
			targets           = ["all:ipv4"]
			notes             = "Externally managed SSH"
		}

		resource "katapult_security_group_rule" "dns" {
			security_group_id = katapult_security_group.my_sg.id
			direction         = "outbound"
			protocol          = "udp"
			ports             = "53"
			targets           = ["all:ipv4", "all:ipv6"]
			notes             = "Externally managed DNS"

			depends_on = [katapult_security_group_rule.ssh]
		}`,
		name,
	)
	reconcileConfig := undent.Stringf(`
		resource "katapult_security_group" "my_sg" {
			name = "%s"
		}

		removed {
			from = katapult_security_group_rule.ssh

			lifecycle {
				destroy = false
			}
		}

		removed {
			from = katapult_security_group_rule.dns

			lifecycle {
				destroy = false
			}
		}`,
		name,
	)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultSecurityGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: externalConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureRuleID(
						"katapult_security_group_rule.ssh", &inboundRuleID,
					),
					testAccCaptureRuleID(
						"katapult_security_group_rule.dns", &outboundRuleID,
					),
				),
			},
			{
				Config:             adoptConfig,
				ExpectNonEmptyPlan: true,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg", "external_rules", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg", "inbound_rules.#", "1",
					),
					resource.TestCheckResourceAttrPtr(
						"katapult_security_group.my_sg",
						"inbound_rules.0.id",
						&inboundRuleID,
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rules.0.direction", "inbound",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rules.0.protocol", "TCP",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rules.0.ports", "22",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rules.0.targets.#", "1",
					),
					resource.TestCheckTypeSetElemAttr(
						"katapult_security_group.my_sg",
						"inbound_rules.0.targets.*", "all:ipv4",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rules.0.notes", "Externally managed SSH",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg", "outbound_rules.#", "1",
					),
					resource.TestCheckResourceAttrPtr(
						"katapult_security_group.my_sg",
						"outbound_rules.0.id",
						&outboundRuleID,
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"outbound_rules.0.direction", "outbound",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"outbound_rules.0.protocol", "UDP",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"outbound_rules.0.ports", "53",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"outbound_rules.0.targets.#", "2",
					),
					resource.TestCheckTypeSetElemAttr(
						"katapult_security_group.my_sg",
						"outbound_rules.0.targets.*", "all:ipv4",
					),
					resource.TestCheckTypeSetElemAttr(
						"katapult_security_group.my_sg",
						"outbound_rules.0.targets.*", "all:ipv6",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"outbound_rules.0.notes", "Externally managed DNS",
					),
				),
			},
			{
				Config:             reconcileConfig,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: reconcileConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg", "inbound_rules.#", "0",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg", "outbound_rules.#", "0",
					),
					testAccCheckKatapultSecurityGroupRuleIDAbsent(
						tt, &inboundRuleID,
					),
					testAccCheckKatapultSecurityGroupRuleIDAbsent(
						tt, &outboundRuleID,
					),
					testAccCheckResourceAbsent(
						"katapult_security_group_rule.ssh",
					),
					testAccCheckResourceAbsent(
						"katapult_security_group_rule.dns",
					),
				),
			},
			{
				Config:   reconcileConfig,
				PlanOnly: true,
			},
		},
	})
}

func TestAccKatapultSecurityGroup_external_rules_disable_to_blocks(t *testing.T) {
	tt := newSecurityGroupCharacterizationTestTools(t)
	name := tt.ResourceName()
	var inboundRuleID, outboundRuleID string

	externalConfig := undent.Stringf(`
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
			notes             = "Externally managed SSH"
		}

		resource "katapult_security_group_rule" "dns" {
			security_group_id = katapult_security_group.my_sg.id
			direction         = "outbound"
			protocol          = "udp"
			ports             = "53"
			targets           = ["all:ipv4", "all:ipv6"]
			notes             = "Externally managed DNS"
			depends_on        = [katapult_security_group_rule.ssh]
		}
	`, name)
	blocksConfig := undent.Stringf(`
		resource "katapult_security_group" "my_sg" {
			name = "%s"

			inbound_rule {
				protocol = "tcp"
				ports    = "2222"
				targets  = ["all:ipv4"]
				notes    = "Externally managed SSH"
			}

			outbound_rule {
				protocol = "udp"
				ports    = "5353"
				targets  = ["all:ipv4", "all:ipv6"]
				notes    = "Externally managed DNS"
			}
		}

		removed {
			from = katapult_security_group_rule.ssh
			lifecycle { destroy = false }
		}

		removed {
			from = katapult_security_group_rule.dns
			lifecycle { destroy = false }
		}
	`, name)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultSecurityGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: externalConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureRuleID("katapult_security_group_rule.ssh", &inboundRuleID),
					testAccCaptureRuleID("katapult_security_group_rule.dns", &outboundRuleID),
				),
			},
			{
				Config:             blocksConfig,
				ExpectNonEmptyPlan: true,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPtr(
						"katapult_security_group.my_sg", "inbound_rule.0.id", &inboundRuleID,
					),
					resource.TestCheckResourceAttr("katapult_security_group.my_sg", "inbound_rule.0.ports", "2222"),
					resource.TestCheckResourceAttrPtr(
						"katapult_security_group.my_sg", "outbound_rule.0.id", &outboundRuleID,
					),
					resource.TestCheckResourceAttr("katapult_security_group.my_sg", "outbound_rule.0.ports", "5353"),
				),
			},
			{
				Config: blocksConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPtr(
						"katapult_security_group.my_sg", "inbound_rule.0.id", &inboundRuleID,
					),
					resource.TestCheckResourceAttr("katapult_security_group.my_sg", "inbound_rule.0.ports", "2222"),
					resource.TestCheckResourceAttrPtr(
						"katapult_security_group.my_sg", "outbound_rule.0.id", &outboundRuleID,
					),
					resource.TestCheckResourceAttr("katapult_security_group.my_sg", "outbound_rule.0.ports", "5353"),
				),
			},
			{
				Config:           blocksConfig,
				PlanOnly:         true,
				ConfigPlanChecks: emptyPostRefreshPlanChecks(),
			},
		},
	})
}

func TestAccKatapultSecurityGroup_out_of_band_deletion(t *testing.T) {
	tt := newSecurityGroupCharacterizationTestTools(t)

	name := tt.ResourceName()
	var groupID string
	var ruleID string

	withRuleConfig := undent.Stringf(`
		resource "katapult_security_group" "my_sg" {
			name           = "%s"
			external_rules = true
		}

		resource "katapult_security_group_rule" "my_rule" {
			security_group_id = katapult_security_group.my_sg.id
			direction         = "inbound"
			protocol          = "tcp"
			ports             = "443"
			targets           = ["all:ipv4"]
			notes             = "HTTPS"
		}`,
		name,
	)
	groupOnlyConfig := undent.Stringf(`
		resource "katapult_security_group" "my_sg" {
			name           = "%s"
			external_rules = true
		}`,
		name,
	)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckKatapultSecurityGroupRuleDestroy(tt),
			testAccCheckKatapultSecurityGroupDestroy(tt),
		),
		Steps: []resource.TestStep{
			{
				Config:             withRuleConfig,
				ExpectNonEmptyPlan: true,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureSecurityGroupAttr(
						"katapult_security_group.my_sg",
						"id", &groupID,
					),
					testAccCaptureAndDeleteSecurityGroupRule(
						tt, "katapult_security_group_rule.my_rule", &ruleID,
					),
				),
			},
			{
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckResourceAbsent(
						"katapult_security_group_rule.my_rule",
					),
					testAccCheckKatapultSecurityGroupExists(
						tt, "katapult_security_group.my_sg",
					),
				),
			},
			{
				Config:             groupOnlyConfig,
				ExpectNonEmptyPlan: true,
				Check: testAccDeleteSecurityGroupByID(
					tt, &groupID,
				),
			},
			{
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
				Check: testAccCheckResourceAbsent(
					"katapult_security_group.my_sg",
				),
			},
		},
	})
}

func TestAccKatapultSecurityGroup_partial_rule_creation_failure(t *testing.T) {
	tt := newSecurityGroupCharacterizationTestTools(t)

	name := tt.ResourceName()
	var groupID string
	failingConfig := undent.Stringf(`
		resource "katapult_security_group" "my_sg" {
			name = "%s"

			inbound_rule {
				protocol = "tcp"
				ports    = "22"
				targets  = ["all:ipv4"]
				notes    = "SSH"
			}

			inbound_rule {
				protocol = "icmp"
				ports    = "443"
				targets  = ["all:ipv4"]
				notes    = "Invalid ICMP rule"
			}
		}`,
		name,
	)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultSecurityGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: failingConfig,
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta("Ports cannot be set with ICMP"),
				),
			},
			{
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCaptureSecurityGroupAttr(
						"katapult_security_group.my_sg",
						"id", &groupID,
					),
					testAccCheckKatapultSecurityGroupRuleCounts(
						tt, "katapult_security_group.my_sg", 1, 0,
					),
				),
			},
			{
				Config:  failingConfig,
				Destroy: true,
				Check: testAccCheckKatapultSecurityGroupIDAbsent(
					tt, &groupID,
				),
			},
		},
	})
}

func testAccCaptureSecurityGroupAttr(
	resourceName string, //nolint:unparam // Supports callers using any SG address.
	attributeName string,
	target *string,
) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		resourceState, ok := state.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found: %s", resourceName)
		}

		value := resourceState.Primary.Attributes[attributeName]
		if value == "" {
			return fmt.Errorf("%s.%s is empty", resourceName, attributeName)
		}

		*target = value

		return nil
	}
}

func testAccCaptureRuleID(
	resourceName string,
	target *string,
) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		resourceState, ok := state.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found: %s", resourceName)
		}

		*target = resourceState.Primary.ID
		if *target == "" {
			return fmt.Errorf("%s has no ID", resourceName)
		}

		return nil
	}
}

func testAccCaptureAndDeleteSecurityGroupRule(
	tt *testTools,
	resourceName string,
	target *string,
) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		resourceState, ok := state.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found: %s", resourceName)
		}

		*target = resourceState.Primary.ID
		if *target == "" {
			return fmt.Errorf("%s has no ID", resourceName)
		}

		response, err := tt.Meta.Core.DeleteSecurityGroupsRulesSecurityGroupRuleWithResponse(
			tt.Ctx,
			core.DeleteSecurityGroupsRulesSecurityGroupRuleJSONRequestBody{
				SecurityGroupRule: core.SecurityGroupRuleLookup{Id: target},
			},
		)
		if err != nil {
			return fmt.Errorf("deleting %s out of band: %w", resourceName, err)
		}
		if response == nil || (response.JSON200 == nil && response.StatusCode() != 204) {
			return fmt.Errorf("unexpected response deleting %s out of band", resourceName)
		}

		return nil
	}
}

func testAccDeleteSecurityGroupByID(
	tt *testTools,
	groupID *string,
) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if *groupID == "" {
			return errors.New("security group ID was not captured")
		}

		response, err := tt.Meta.Core.DeleteSecurityGroupWithResponse(
			tt.Ctx,
			core.DeleteSecurityGroupJSONRequestBody{
				SecurityGroup: core.SecurityGroupLookup{Id: groupID},
			},
		)
		if err != nil {
			return fmt.Errorf("deleting security group out of band: %w", err)
		}
		if response == nil || (response.JSON200 == nil && response.StatusCode() != 204) {
			return errors.New("unexpected response deleting security group out of band")
		}

		return nil
	}
}

func testAccCheckResourceAbsent(resourceName string) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		if _, ok := state.RootModule().Resources[resourceName]; ok {
			return fmt.Errorf("resource remains in state: %s", resourceName)
		}

		return nil
	}
}

func testAccCheckKatapultSecurityGroupRuleCounts(
	tt *testTools,
	groupResourceName string,
	wantInbound int,
	wantOutbound int,
) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		groupState, ok := state.RootModule().Resources[groupResourceName]
		if !ok {
			return fmt.Errorf("resource not found: %s", groupResourceName)
		}

		rules, err := getAllSecurityGroupRules(tt.Ctx, tt.Meta, groupState.Primary.ID)
		if err != nil {
			return err
		}
		inbound, outbound := 0, 0
		for _, rule := range rules {
			switch rule.Direction {
			case securityGroupDirectionInbound:
				inbound++
			case securityGroupDirectionOutbound:
				outbound++
			}
		}

		if inbound != wantInbound || outbound != wantOutbound {
			return fmt.Errorf(
				"security group %s has %d inbound and %d outbound rules, want %d and %d",
				groupState.Primary.ID,
				inbound,
				outbound,
				wantInbound,
				wantOutbound,
			)
		}

		return nil
	}
}

func testAccCheckKatapultSecurityGroupRuleIDAbsent(
	tt *testTools,
	ruleID *string,
) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if *ruleID == "" {
			return errors.New("security group rule ID was not captured")
		}

		response, err := tt.Meta.Core.GetSecurityGroupsRulesSecurityGroupRuleWithResponse(
			tt.Ctx,
			&core.GetSecurityGroupsRulesSecurityGroupRuleParams{SecurityGroupRuleId: ruleID},
		)
		if errors.Is(err, core.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if response != nil && response.JSON404 != nil {
			return nil
		}

		return fmt.Errorf("security group rule still exists: %s", *ruleID)
	}
}

func testAccCheckKatapultSecurityGroupIDAbsent(
	tt *testTools,
	groupID *string,
) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if *groupID == "" {
			return errors.New("security group ID was not captured")
		}

		response, err := tt.Meta.Core.GetSecurityGroupWithResponse(
			tt.Ctx,
			&core.GetSecurityGroupParams{SecurityGroupId: groupID},
		)
		if errors.Is(err, core.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if response != nil && response.JSON404 != nil {
			return nil
		}

		return fmt.Errorf("security group still exists: %s", *groupID)
	}
}
