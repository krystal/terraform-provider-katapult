package v6provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
)

func TestAccKatapultLoadBalancerRule_minimal(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultLoadBalancerDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "my_lb" {
					  name = "%s"
					}
					
					resource "katapult_load_balancer_rule" "my_rule" {
						load_balancer_id = katapult_load_balancer.my_lb.id
						destination_port = 8080
						listen_port = 80
						protocol = "HTTP"
						passthrough_ssl = false
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerRuleExists(
						tt, "katapult_load_balancer_rule.my_rule",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.my_rule",
						"destination_port",
						"8080",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.my_rule",
						"listen_port",
						"80",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.my_rule",
						"protocol",
						"HTTP",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.my_rule",
						"passthrough_ssl",
						"false",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.my_rule",
						"check_enabled",
						"false",
					),
				),
			},
			{
				ResourceName:      "katapult_load_balancer_rule.my_rule",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultLoadBalancerRule_update(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultLoadBalancerDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "my_lb" {
					  name = "%s"
					}
					
					resource "katapult_load_balancer_rule" "my_rule" {
						load_balancer_id = katapult_load_balancer.my_lb.id
						destination_port = 8080
						listen_port = 80
						protocol = "HTTP"
						passthrough_ssl = false
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerRuleExists(
						tt, "katapult_load_balancer_rule.my_rule",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "my_lb" {
						name= "%s"
					}

					resource "katapult_load_balancer_rule" "my_rule" {
						load_balancer_id = katapult_load_balancer.my_lb.id
						destination_port = 8080
						listen_port = 443
						protocol = "HTTPS"
						passthrough_ssl = true
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerRuleExists(
						tt,
						"katapult_load_balancer_rule.my_rule",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.my_rule",
						"listen_port",
						"443",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.my_rule",
						"protocol",
						"HTTPS",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.my_rule",
						"passthrough_ssl",
						"true",
					),
				),
			},
			{
				ResourceName:      "katapult_load_balancer_rule.my_rule",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultLoadBalancerRule_invalid(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultLoadBalancerDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "my_lb" {
					  name = "%s"
					}
					
					resource "katapult_load_balancer_rule" "my_rule" {
						load_balancer_id = katapult_load_balancer.my_lb.id
						destination_port = 8080
						listen_port = 443
						protocol = "HTTPS"
						passthrough_ssl = false
						check_enabled = true
						check_protocol = "HTTPS"
						check_path = "/"
					}`,
					name,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						"check_path cannot be set " +
							"if check_protocol is not HTTP",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "my_lb" {
					  name = "%s"
					}
					
					resource "katapult_load_balancer_rule" "my_rule" {
						load_balancer_id = katapult_load_balancer.my_lb.id
						destination_port = 8080
						listen_port = 80
						protocol = "HTTP"
						passthrough_ssl = false
						check_enabled = true
						check_protocol = "HTTPS"
						check_http_statuses = "2"
					}`,
					name,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						"check_http_statuses cannot be set " +
							"if check_protocol is not HTTP",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "my_lb" {
					  name = "%s"
					}
					
					resource "katapult_load_balancer_rule" "my_rule" {
						load_balancer_id = katapult_load_balancer.my_lb.id
						destination_port = 8080
						listen_port = 80
						protocol = "HTTP"
						passthrough_ssl = false
						certificate_ids = ["cert_123"]
					}`,
					name,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						"certificate_ids cannot be set " +
							"if protocol is not HTTPS",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "my_lb" {
					  name = "%s"
					}
					
					resource "katapult_load_balancer_rule" "my_rule" {
						load_balancer_id = katapult_load_balancer.my_lb.id
						destination_port = 8080
						listen_port = 80
						protocol = "HTTP"
						passthrough_ssl = true
					}`,
					name,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						"passthrough_ssl cannot be set " +
							"if protocol is not HTTPS",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "my_lb" {
					  name = "%s"
					}
					
					resource "katapult_load_balancer_rule" "my_rule" {
						load_balancer_id = katapult_load_balancer.my_lb.id
						destination_port = 8080
						listen_port = 80
						protocol = "ICMP"
						passthrough_ssl = false
					}`,
					name,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						"protocol value must be one of: " +
							`["HTTP" "HTTPS" "TCP"], ` +
							`got: "ICMP"`,
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "my_lb" {
					  name = "%s"
					}
					
					resource "katapult_load_balancer_rule" "my_rule" {
						load_balancer_id = katapult_load_balancer.my_lb.id
						destination_port = 8080
						listen_port = 80
						protocol = "HTTP"
						passthrough_ssl = false
						check_enabled = true
						check_protocol = "ICMP"
					}`,
					name,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						"check_protocol value must be one of: " +
							`["HTTP" "HTTPS" "TCP"]`,
					),
				),
			},
		},
	})
}

//
// Helpers
//

func testAccCheckKatapultLoadBalancerRuleExists(
	tt *testTools,
	res string,
) resource.TestCheckFunc {
	m := tt.Meta

	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("resource not found: %s", res)
		}

		lbr, _, err := m.Core.LoadBalancerRules.GetByID(
			tt.Ctx, rs.Primary.ID,
		)
		if err != nil {
			return err
		}

		return resource.TestCheckResourceAttr(
			res,
			"algorithm",
			string(lbr.Algorithm),
		)(s)
	}
}
