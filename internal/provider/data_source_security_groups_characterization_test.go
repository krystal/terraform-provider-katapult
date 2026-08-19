package provider

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/jimeh/undent"
)

type expectedSecurityGroupRuleState struct {
	direction string
	protocol  string
	ports     string
	targets   []string
	notes     string
}

func TestAccKatapultDataSourceSecurityGroups_include_rules(t *testing.T) {
	tt := newSecurityGroupCharacterizationTestTools(t)

	name := tt.ResourceName()
	config := undent.Stringf(`
		resource "katapult_security_group" "inbound" {
			name = "%s"

			inbound_rule {
				protocol = "tcp"
				ports    = "22"
				targets  = ["all:ipv4"]
				notes    = "SSH"
			}
		}

		resource "katapult_security_group" "outbound" {
			name = "%s"

			outbound_rule {
				protocol = "udp"
				ports    = "53"
				targets  = ["all:ipv4", "all:ipv6"]
				notes    = "DNS"
			}

			depends_on = [katapult_security_group.inbound]
		}

		data "katapult_security_groups" "all" {
			include_rules = true

			depends_on = [katapult_security_group.outbound]
		}`,
		name+"-inbound", name+"-outbound",
	)

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultSecurityGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckSecurityGroupCollectionRules(
						"data.katapult_security_groups.all",
						name+"-inbound",
						&expectedSecurityGroupRuleState{
							direction: "inbound",
							protocol:  "TCP",
							ports:     "22",
							targets:   []string{"all:ipv4"},
							notes:     "SSH",
						},
						nil,
					),
					testAccCheckSecurityGroupCollectionRules(
						"data.katapult_security_groups.all",
						name+"-outbound",
						nil,
						&expectedSecurityGroupRuleState{
							direction: "outbound",
							protocol:  "UDP",
							ports:     "53",
							targets:   []string{"all:ipv4", "all:ipv6"},
							notes:     "DNS",
						},
					),
				),
			},
		},
	})
}

func testAccCheckSecurityGroupCollectionRules(
	resourceName string,
	groupName string,
	wantInbound *expectedSecurityGroupRuleState,
	wantOutbound *expectedSecurityGroupRuleState,
) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		resourceState, ok := state.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found: %s", resourceName)
		}

		count, err := strconv.Atoi(
			resourceState.Primary.Attributes["security_groups.#"],
		)
		if err != nil {
			return fmt.Errorf("reading %s security group count: %w", resourceName, err)
		}

		for i := 0; i < count; i++ {
			prefix := fmt.Sprintf("security_groups.%d.", i)
			if resourceState.Primary.Attributes[prefix+"name"] != groupName {
				continue
			}

			wantInboundCount := 0
			if wantInbound != nil {
				wantInboundCount = 1
			}
			inbound := resourceState.Primary.Attributes[prefix+"inbound_rules.#"]
			if inbound != strconv.Itoa(wantInboundCount) {
				return fmt.Errorf(
					"%s has %s inbound rules, want %d",
					groupName, inbound, wantInboundCount,
				)
			}

			wantOutboundCount := 0
			if wantOutbound != nil {
				wantOutboundCount = 1
			}
			outbound := resourceState.Primary.Attributes[prefix+"outbound_rules.#"]
			if outbound != strconv.Itoa(wantOutboundCount) {
				return fmt.Errorf(
					"%s has %s outbound rules, want %d",
					groupName, outbound, wantOutboundCount,
				)
			}

			if wantInbound != nil {
				if err := testAccCheckSecurityGroupCollectionRule(
					resourceState.Primary.Attributes,
					prefix+"inbound_rules.0.",
					*wantInbound,
				); err != nil {
					return fmt.Errorf("%s inbound rule: %w", groupName, err)
				}
			}

			if wantOutbound != nil {
				if err := testAccCheckSecurityGroupCollectionRule(
					resourceState.Primary.Attributes,
					prefix+"outbound_rules.0.",
					*wantOutbound,
				); err != nil {
					return fmt.Errorf("%s outbound rule: %w", groupName, err)
				}
			}

			return nil
		}

		return fmt.Errorf("security group %q not found in %s", groupName, resourceName)
	}
}

func testAccCheckSecurityGroupCollectionRule(
	attributes map[string]string,
	prefix string,
	want expectedSecurityGroupRuleState,
) error {
	if attributes[prefix+"id"] == "" {
		return errors.New("ID is empty")
	}

	for attribute, expected := range map[string]string{
		"direction": want.direction,
		"protocol":  want.protocol,
		"ports":     want.ports,
		"notes":     want.notes,
	} {
		if actual := attributes[prefix+attribute]; actual != expected {
			return fmt.Errorf(
				"%s is %q, want %q", attribute, actual, expected,
			)
		}
	}

	targetCount, err := strconv.Atoi(attributes[prefix+"targets.#"])
	if err != nil {
		return fmt.Errorf("reading target count: %w", err)
	}
	if targetCount != len(want.targets) {
		return fmt.Errorf("has %d targets, want %d", targetCount, len(want.targets))
	}

	targets := map[string]bool{}
	for attribute, value := range attributes {
		if strings.HasPrefix(attribute, prefix+"targets.") &&
			attribute != prefix+"targets.#" {
			targets[value] = true
		}
	}
	for _, target := range want.targets {
		if !targets[target] {
			return fmt.Errorf("target %q is missing", target)
		}
	}

	return nil
}
