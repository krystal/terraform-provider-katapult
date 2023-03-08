package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceSecurityGroupRule_by_id(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultIPDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"
						external_rules = true
					}

					resource "katapult_security_group_rule" "my_rule" {
						security_group_id = katapult_security_group.my_sg.id
						direction = "inbound"
						protocol = "tcp"
						ports = "22"
						targets = ["93.89.203.0/24"]
						notes = "Trusted SSH"
					}

					data "katapult_security_group_rule" "my_rule" {
						id = katapult_security_group_rule.my_rule.id
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupRuleExists(
						tt, "data.katapult_security_group_rule.my_rule",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group_rule.my_rule",
						"direction",
						"katapult_security_group_rule.my_rule",
						"direction",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group_rule.my_rule", "protocol",
						"katapult_security_group_rule.my_rule", "protocol",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group_rule.my_rule", "ports",
						"katapult_security_group_rule.my_rule", "ports",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group_rule.my_rule", "targets",
						"katapult_security_group_rule.my_rule", "targets",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_security_group_rule.my_rule", "notes",
						"katapult_security_group_rule.my_rule", "notes",
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceSecurityGroupRule_not_found(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultSecurityGroupRuleDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					data "katapult_security_group_rule" "my_rule" {
						id = "sgr_thisdoesnotexist"
					}`,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						"security_group_rule_not_found: " +
							"No security group rule was found matching any " +
							"of the criteria provided in the arguments",
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceSecurityGroupRule_blank(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultSecurityGroupRuleDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					data "katapult_security_group_rule" "my_rule" {

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
