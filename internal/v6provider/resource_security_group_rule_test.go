package v6provider

import (
	"errors"
	"fmt"
	"regexp"
	"testing"

	"github.com/dnaeon/go-vcr/recorder"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/next/core"
)

func TestAccKatapultSecurityGroupRule_example(t *testing.T) {
	if vcrMode() == recorder.ModeReplaying {
		t.Skip("example based tests are not supported in replay mode")
	}

	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultSecurityGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: exampleResourceConfig(
					t, "katapult_security_group_rule",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupRuleExists(
						tt, "katapult_security_group_rule.minimal",
					),
					testAccCheckKatapultSecurityGroupRuleExists(
						tt, "katapult_security_group_rule.http",
					),
					testAccCheckKatapultSecurityGroupRuleExists(
						tt, "katapult_security_group_rule.ssh",
					),
					testAccCheckKatapultSecurityGroupRuleExists(
						tt, "katapult_security_group_rule.range",
					),
					testAccCheckKatapultSecurityGroupRuleExists(
						tt, "katapult_security_group_rule.range_all_ports",
					),
					testAccCheckKatapultSecurityGroupRuleExists(
						tt, "katapult_security_group_rule.smtp",
					),
					testAccCheckKatapultSecurityGroupRuleExists(
						tt, "katapult_security_group_rule.http_out",
					),
				),
			},
		},
	})
}

func TestAccKatapultSecurityGroupRule_minimal(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultSecurityGroupRuleDestroy(tt),
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
						targets = []
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupRuleExists(
						tt, "katapult_security_group_rule.my_rule",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group_rule.my_rule",
						"protocol", "tcp",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group_rule.my_rule",
						"ports", "",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group_rule.my_rule",
						"notes", "",
					),
				),
			},
			{
				ResourceName:            "katapult_security_group_rule.my_rule",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"protocol"},
			},
		},
	})
}

func TestAccKatapultSecurityGroupRule_update(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultSecurityGroupRuleDestroy(tt),
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
						ports = "80"
						targets = ["all:ipv4"]
						notes = "HTTP"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupRuleExists(
						tt, "katapult_security_group_rule.my_rule",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group_rule.my_rule",
						"protocol", "tcp",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"
						external_rules = true
					}

					resource "katapult_security_group_rule" "my_rule" {
						security_group_id = katapult_security_group.my_sg.id
						direction = "outbound"
						protocol = "udp"
						ports = "443"
						targets = ["all:ipv6"]
						notes = "QUIC"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupRuleExists(
						tt, "katapult_security_group_rule.my_rule",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group_rule.my_rule",
						"protocol", "udp",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"
						external_rules = true
					}

					resource "katapult_security_group_rule" "my_rule" {
						security_group_id = katapult_security_group.my_sg.id
						direction = "inbound"
						protocol = "icmp"
						ports = "443"
						targets = ["10.0.0.0/24"]
						notes = "PING"
					}`,
					name,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta("Ports cannot be set with ICMP"),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"
						external_rules = true
					}

					resource "katapult_security_group_rule" "my_rule" {
						security_group_id = katapult_security_group.my_sg.id
						direction = "inbound"
						protocol = "icmp"
						targets = ["10.0.0.0/24"]
						notes = "PING"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupRuleExists(
						tt, "katapult_security_group_rule.my_rule",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group_rule.my_rule",
						"protocol", "icmp",
					),
				),
			},
			{
				ResourceName:            "katapult_security_group_rule.my_rule",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"protocol"},
			},
		},
	})
}

func TestAccKatapultSecurityGroupRule_tcp(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultSecurityGroupRuleDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"
						external_rules = true
					}

					resource "katapult_security_group_rule" "http" {
						security_group_id = katapult_security_group.my_sg.id
						direction = "inbound"
						protocol = "tcp"
						ports = "80"
						targets = ["all:ipv4"]
						notes = "HTTP"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupRuleExists(
						tt, "katapult_security_group_rule.http",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group_rule.http",
						"protocol", "tcp",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"
						external_rules = true
					}

					resource "katapult_security_group_rule" "http" {
						security_group_id = katapult_security_group.my_sg.id
						direction = "inbound"
						protocol = "TCP"
						ports = "80,433"
						targets = ["all:ipv4", "all:ipv6"]
						notes = "HTTP & HTTPS"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupRuleExists(
						tt, "katapult_security_group_rule.http",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group_rule.http",
						"protocol", "tcp",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"
						external_rules = true
					}

					resource "katapult_security_group_rule" "http" {
						security_group_id = katapult_security_group.my_sg.id
						direction = "inbound"
						protocol = "tcp"
						ports = "80,433"
						targets = ["all:ipv4", "all:ipv6"]
						notes = "HTTP & HTTPS"
					}

					resource "katapult_security_group_rule" "ssh" {
						security_group_id = katapult_security_group.my_sg.id
						direction = "inbound"
						protocol = "TCP"
						ports = "22"
						targets = ["all:ipv4", "all:ipv6"]
						notes = "SSH"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupRuleExists(
						tt, "katapult_security_group_rule.http",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group_rule.http",
						"protocol", "tcp",
					),
					testAccCheckKatapultSecurityGroupRuleExists(
						tt, "katapult_security_group_rule.ssh",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group_rule.ssh",
						"protocol", "TCP",
					),
				),
			},
			{
				ResourceName:            "katapult_security_group_rule.http",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"protocol"},
			},
		},
	})
}

func TestAccKatapultSecurityGroupRule_udp(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultSecurityGroupRuleDestroy(tt),
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
						protocol = "udp"
						ports = "443"
						targets = ["10.0.0.1/24"]
						notes = "QUIC"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupRuleExists(
						tt, "katapult_security_group_rule.my_rule",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group_rule.my_rule",
						"protocol", "udp",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"
						external_rules = true
					}

					resource "katapult_security_group_rule" "my_rule" {
						security_group_id = katapult_security_group.my_sg.id
						direction = "outbound"
						protocol = "UDP"
						ports = "3000-4999"
						targets = ["all:ipv4", "all:ipv6"]
						notes = "Custom"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupRuleExists(
						tt, "katapult_security_group_rule.my_rule",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group_rule.my_rule",
						"protocol", "udp",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"
						external_rules = true
					}

					resource "katapult_security_group_rule" "my_rule" {
						security_group_id = katapult_security_group.my_sg.id
						direction = "inbound"
						protocol = "udp"
						ports = "3000-4999"
						targets = ["all:ipv4", "all:ipv6"]
						notes = "Custom"
					}

					resource "katapult_security_group_rule" "quic" {
						security_group_id = katapult_security_group.my_sg.id
						direction = "inbound"
						protocol = "UDP"
						ports = "433"
						targets = ["all:ipv4", "all:ipv6"]
						notes = "QUIC"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupRuleExists(
						tt, "katapult_security_group_rule.my_rule",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group_rule.my_rule",
						"protocol", "udp",
					),
					testAccCheckKatapultSecurityGroupRuleExists(
						tt, "katapult_security_group_rule.quic",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group_rule.quic",
						"protocol", "UDP",
					),
				),
			},
			{
				ResourceName:            "katapult_security_group_rule.my_rule",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"protocol"},
			},
		},
	})
}

func TestAccKatapultSecurityGroupRule_icmp(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultSecurityGroupRuleDestroy(tt),
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
						protocol = "icmp"
						targets = ["10.0.0.1/24"]
						notes = "ping"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupRuleExists(
						tt, "katapult_security_group_rule.my_rule",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group_rule.my_rule",
						"protocol", "icmp",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"
						external_rules = true
					}

					resource "katapult_security_group_rule" "my_rule" {
						security_group_id = katapult_security_group.my_sg.id
						direction = "outbound"
						protocol = "icmp"
						targets = ["all:ipv4"]
						notes = "ping out"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupRuleExists(
						tt, "katapult_security_group_rule.my_rule",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group_rule.my_rule",
						"protocol", "icmp",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"
						external_rules = true
					}

					resource "katapult_security_group_rule" "my_rule" {
						security_group_id = katapult_security_group.my_sg.id
						direction = "outbound"
						protocol = "icmp"
						ports = "7"
						targets = ["all:ipv4", "all:ipv6"]
						notes = "ping out"
					}`,
					name,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta("Ports cannot be set with ICMP"),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"
						external_rules = true
					}

					resource "katapult_security_group_rule" "my_rule" {
						security_group_id = katapult_security_group.my_sg.id
						direction = "outbound"
						protocol = "icmp"
						targets = ["all:ipv4"]
						notes = "ping out"
					}

					resource "katapult_security_group_rule" "pingme" {
						security_group_id = katapult_security_group.my_sg.id
						direction = "inbound"
						protocol = "ICMP"
						targets = ["all:ipv4", "all:ipv6"]
						notes = "ping me"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupRuleExists(
						tt, "katapult_security_group_rule.my_rule",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group_rule.my_rule",
						"protocol", "icmp",
					),
					testAccCheckKatapultSecurityGroupRuleExists(
						tt, "katapult_security_group_rule.pingme",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group_rule.pingme",
						"protocol", "ICMP",
					),
				),
			},
			{
				ResourceName:            "katapult_security_group_rule.my_rule",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"protocol"},
			},
		},
	})
}

func TestAccKatapultSecurityGroupRule_invalid(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultSecurityGroupRuleDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"
						external_rules = true
					}

					resource "katapult_security_group_rule" "my_rule" {
						security_group_id = katapult_security_group.my_sg.id
						direction = "upwards"
						protocol = "tcp"
						targets = []
					}`,
					name,
				),
				ExpectError: regexp.MustCompile(
					"Invalid Attribute Value Match",
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"
						external_rules = true
					}

					resource "katapult_security_group_rule" "my_rule" {
						security_group_id = katapult_security_group.my_sg.id
						direction = "inbound"
						protocol = "grpc"
						targets = []
					}`,
					name,
				),
				ExpectError: regexp.MustCompile(
					"Invalid Attribute Value Match",
				),
			},
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
						targets = [""]
					}`,
					name,
				),
				ExpectError: regexp.MustCompile("Invalid Attribute Value Length"),
			},
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
						targets = [null]
					}`,
					name,
				),
				ExpectError: regexp.MustCompile("Null Set Value"),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"
						external_rules = true
					}

					resource "katapult_security_group_rule" "my_rule" {
						security_group_id = katapult_security_group.my_sg.id
						direction = "inbound"
						protocol = "icmp"
						ports = "80"
						targets = []
					}`,
					name,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta("Ports cannot be set with ICMP"),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"
						allow_all_inbound = true
					}

					resource "katapult_security_group_rule" "my_rule" {
						security_group_id = katapult_security_group.my_sg.id
						direction = "inbound"
						protocol = "tcp"
						targets = []
					}`,
					name,
				),
				ExpectError: regexp.MustCompile(
					"Security group cannot have inbound rules",
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"
						allow_all_outbound = true
					}

					resource "katapult_security_group_rule" "my_rule" {
						security_group_id = katapult_security_group.my_sg.id
						direction = "outbound"
						protocol = "tcp"
						targets = []
					}`,
					name,
				),
				ExpectError: regexp.MustCompile(
					"Security group cannot have outbound rules",
				),
			},
		},
	})
}

func testAccCheckKatapultSecurityGroupRuleDestroy(
	tt *testTools,
) resource.TestCheckFunc {
	m := tt.Meta

	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "katapult_security_group_rule" {
				continue
			}

			attempts := 1
			if tt.Recorder != nil && tt.Recorder.Mode() == recorder.ModeReplaying {
				attempts = 100
			}
			destroyed := false
			for range attempts {
				id := rs.Primary.ID
				response, err := m.Core.GetSecurityGroupsRulesSecurityGroupRuleWithResponse(
					tt.Ctx,
					&core.GetSecurityGroupsRulesSecurityGroupRuleParams{SecurityGroupRuleId: &id},
				)
				if errors.Is(err, core.ErrNotFound) || response != nil && response.JSON404 != nil {
					destroyed = true
					break
				}
				if err != nil {
					return err
				}
				if attempts == 1 {
					return fmt.Errorf("katapult_security_group_rule %s was not destroyed", id)
				}
			}
			if !destroyed {
				return fmt.Errorf("katapult_security_group_rule %s was not destroyed", rs.Primary.ID)
			}
		}

		return nil
	}
}

func testAccCheckKatapultSecurityGroupRuleExists(
	tt *testTools,
	res string,
) resource.TestCheckFunc {
	m := tt.Meta

	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("resource not found: %s", res)
		}

		id := rs.Primary.ID
		response, err := m.Core.GetSecurityGroupsRulesSecurityGroupRuleWithResponse(
			tt.Ctx,
			&core.GetSecurityGroupsRulesSecurityGroupRuleParams{SecurityGroupRuleId: &id},
		)
		if err != nil {
			return err
		}
		if response == nil || response.JSON200 == nil || response.JSON200.SecurityGroupRule.Id == nil {
			return fmt.Errorf("security group rule %s not found", id)
		}

		return resource.TestCheckResourceAttr(res, "id", *response.JSON200.SecurityGroupRule.Id)(s)
	}
}
