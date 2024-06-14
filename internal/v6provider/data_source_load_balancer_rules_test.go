package v6provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceLoadBalancerRules_full(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultLoadBalancerDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "main" {
						name = "%s"
					}

					resource "katapult_load_balancer_rule" "my_rule" {
						load_balancer_id = katapult_load_balancer.main.id
						destination_port = 8080
						listen_port = 80
						protocol = "HTTP"
						backend_ssl = true
						passthrough_ssl = false
						proxy_protocol = true
						check_enabled = false
						check_fall = 3
						check_interval = 30
						check_http_statuses = "23"
						check_path = "/healthz"
						check_protocol = "HTTP"
						check_rise = 3
						check_timeout = 15
					}

					data "katapult_load_balancer_rules" "src" {
					depends_on = [katapult_load_balancer_rule.my_rule]
						load_balancer_id = katapult_load_balancer.main.id
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.katapult_load_balancer_rules.src",
						"rules.#",
						"1",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_load_balancer_rules.src",
						"rules.0.destination_port",
						"8080",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_load_balancer_rules.src",
						"rules.0.listen_port",
						"80",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_load_balancer_rules.src",
						"rules.0.protocol",
						"HTTP",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_load_balancer_rules.src",
						"rules.0.backend_ssl",
						"true",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_load_balancer_rules.src",
						"rules.0.passthrough_ssl",
						"false",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_load_balancer_rules.src",
						"rules.0.proxy_protocol",
						"true",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_load_balancer_rules.src",
						"rules.0.check_enabled",
						"false",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_load_balancer_rules.src",
						"rules.0.check_fall",
						"3",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_load_balancer_rules.src",
						"rules.0.check_interval",
						"30",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_load_balancer_rules.src",
						"rules.0.check_http_statuses",
						"23",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_load_balancer_rules.src",
						"rules.0.check_path",
						"/healthz",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_load_balancer_rules.src",
						"rules.0.check_protocol",
						"HTTP",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_load_balancer_rules.src",
						"rules.0.check_rise",
						"3",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_load_balancer_rules.src",
						"rules.0.check_timeout",
						"15",
					),
				),
			},
		},
	})
}
