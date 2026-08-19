package provider

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceSecurityGroups_include_rules(t *testing.T) {
	tt := newTestTools(t)

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
						name+"-inbound", 1, 0, "TCP", "",
					),
					testAccCheckSecurityGroupCollectionRules(
						"data.katapult_security_groups.all",
						name+"-outbound", 0, 1, "", "UDP",
					),
				),
			},
		},
	})
}

func testAccCheckSecurityGroupCollectionRules(
	resourceName string,
	groupName string,
	wantInbound int,
	wantOutbound int,
	wantInboundProtocol string,
	wantOutboundProtocol string,
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

			inbound := resourceState.Primary.Attributes[prefix+"inbound_rules.#"]
			if inbound != strconv.Itoa(wantInbound) {
				return fmt.Errorf(
					"%s has %s inbound rules, want %d",
					groupName, inbound, wantInbound,
				)
			}

			outbound := resourceState.Primary.Attributes[prefix+"outbound_rules.#"]
			if outbound != strconv.Itoa(wantOutbound) {
				return fmt.Errorf(
					"%s has %s outbound rules, want %d",
					groupName, outbound, wantOutbound,
				)
			}

			if wantInboundProtocol != "" {
				inboundPrefix := prefix + "inbound_rules.0."
				if id := resourceState.Primary.Attributes[inboundPrefix+"id"]; id == "" {
					return fmt.Errorf("%s inbound rule has no ID", groupName)
				}
				if direction := resourceState.Primary.Attributes[inboundPrefix+"direction"]; direction != "inbound" {
					return fmt.Errorf(
						"%s inbound rule has direction %q", groupName, direction,
					)
				}
				protocol := resourceState.Primary.Attributes[inboundPrefix+"protocol"]
				if protocol != wantInboundProtocol {
					return fmt.Errorf(
						"%s inbound rule has protocol %q, want %q",
						groupName, protocol, wantInboundProtocol,
					)
				}
			}

			if wantOutboundProtocol != "" {
				outboundPrefix := prefix + "outbound_rules.0."
				if id := resourceState.Primary.Attributes[outboundPrefix+"id"]; id == "" {
					return fmt.Errorf("%s outbound rule has no ID", groupName)
				}
				if direction := resourceState.Primary.Attributes[outboundPrefix+"direction"]; direction != "outbound" {
					return fmt.Errorf(
						"%s outbound rule has direction %q", groupName, direction,
					)
				}
				protocol := resourceState.Primary.Attributes[outboundPrefix+"protocol"]
				if protocol != wantOutboundProtocol {
					return fmt.Errorf(
						"%s outbound rule has protocol %q, want %q",
						groupName, protocol, wantOutboundProtocol,
					)
				}
			}

			return nil
		}

		return fmt.Errorf("security group %q not found in %s", groupName, resourceName)
	}
}
