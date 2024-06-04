package v6provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceLoadBalancerRules_minimal(t *testing.T) {
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
						passthrough_ssl = false
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerExists(
						tt, "katapult_load_balancer.main",
					),
				),
			},
			// setup the data source in a second step
			// to ensure the rule is created.
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
					passthrough_ssl = false
				}

				data "katapult_load_balancer_rules" "src" {
				  load_balancer_id = katapult_load_balancer.main.id
				}
				`),
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
						"rules.0.passthrough_ssl",
						"false",
					),
				),
			},
		},
	})
}
