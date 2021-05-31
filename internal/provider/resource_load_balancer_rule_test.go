package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/core"
)

func TestAccKatapultLoadBalancerRule_basic(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName("rule-basic")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultLoadBalancerDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "web" {
					  name = "%s"
					}

					resource "katapult_load_balancer_rule" "http" {
					  load_balancer_id = katapult_load_balancer.web.id
					  protocol = "http"
					  listen_port = 80
					  destination_port = 8080
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerRuleExists(
						tt, "katapult_load_balancer_rule.http",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.http",
						"protocol", string(core.HTTPProtocol),
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.http",
						"listen_port", "80",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.http",
						"destination_port", "8080",
					),
				),
			},
			{
				ResourceName:      "katapult_load_balancer_rule.http",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultLoadBalancerRule_basic_healthcheck(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName("rule-basic-healthcheck")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultLoadBalancerDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "web" {
						name = "%s"
					}

					resource "katapult_load_balancer_rule" "http" {
						load_balancer_id = katapult_load_balancer.web.id
						protocol = "http"
						listen_port = 80
						destination_port = 8080
						healthcheck {
							enabled = true
						}
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerRuleExists(
						tt, "katapult_load_balancer_rule.http",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.http",
						"protocol", string(core.HTTPProtocol),
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.http",
						"listen_port", "80",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.http",
						"destination_port", "8080",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.http",
						"healthcheck.0.enabled", "true",
					),
				),
			},
			{
				ResourceName:      "katapult_load_balancer_rule.http",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultLoadBalancerRule_full_healthcheck(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName("rule-full-healthcheck")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultLoadBalancerDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "web" {
						name = "%s"
					}

					resource "katapult_load_balancer_rule" "http" {
						load_balancer_id = katapult_load_balancer.web.id
						protocol = "http"
						listen_port = 80
						destination_port = 8080
						healthcheck {
							enabled = true
							protocol = "HTTP"
							path = "/healthz"
							interval = 5
							healthy = 5
							unhealthy = 3
							timeout = 13
						}
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerRuleExists(
						tt, "katapult_load_balancer_rule.http",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.http",
						"protocol", string(core.HTTPProtocol),
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.http",
						"listen_port", "80",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.http",
						"destination_port", "8080",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.http",
						"healthcheck.0.enabled", "true",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.http",
						"healthcheck.0.protocol", "HTTP",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.http",
						"healthcheck.0.path", "/healthz",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.http",
						"healthcheck.0.interval", "5",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.http",
						"healthcheck.0.healthy", "5",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.http",
						"healthcheck.0.unhealthy", "3",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.http",
						"healthcheck.0.timeout", "13",
					),
				),
			},
			{
				ResourceName:      "katapult_load_balancer_rule.http",
				ImportState:       true,
				ImportStateVerify: true,
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

		lb, _, err := m.Core.LoadBalancerRules.GetByID(
			tt.Ctx, rs.Primary.ID,
		)
		if err != nil {
			return err
		}

		return resource.TestCheckResourceAttr(res, "id", lb.ID)(s)
	}
}
