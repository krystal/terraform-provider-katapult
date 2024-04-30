package v6provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceLoadBalancers_basic(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultLoadBalancerDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "first" {
					  name = "%s-m"
					}

					resource "katapult_load_balancer_rule" "first_rule" {
						load_balancer_id = katapult_load_balancer.first.id
						destination_port = 8080
						listen_port = 80
						protocol = "HTTP"
						passthrough_ssl = false
					}

					resource "katapult_load_balancer" "second" {
						name = "%s-t"
						depends_on = [katapult_load_balancer.first]
					  }

					resource "katapult_load_balancer_rule" "second_rule" {
						load_balancer_id = katapult_load_balancer.second.id
						destination_port = 8080
						listen_port = 80
						protocol = "HTTP"
						passthrough_ssl = false
					}

					`,
					name,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerExists(
						tt, "katapult_load_balancer.first",
					),
					testAccCheckKatapultLoadBalancerExists(
						tt, "katapult_load_balancer.second",
					),
				),
			},
			{
				Config: undent.String((`
				data "katapult_load_balancers" "src" {}

				`)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.katapult_load_balancers.src",
						"load_balancers.#",
						"2",
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceLoadBalancers_rules(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultLoadBalancerDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "first" {
					  name = "%s-m"
					}

					resource "katapult_load_balancer_rule" "first_rule" {
						load_balancer_id = katapult_load_balancer.first.id
						destination_port = 8080
						listen_port = 80
						protocol = "HTTP"
						passthrough_ssl = false
					}

					resource "katapult_load_balancer" "second" {
					  name = "%s-t"
					  depends_on = [katapult_load_balancer.first]
					}

					resource "katapult_load_balancer_rule" "second_rule" {
						load_balancer_id = katapult_load_balancer.second.id
						destination_port = 8443
						listen_port = 443
						protocol = "HTTP"
						passthrough_ssl = false
					}
					`,
					name,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerExists(
						tt, "katapult_load_balancer.first",
					),
					testAccCheckKatapultLoadBalancerExists(
						tt, "katapult_load_balancer.second",
					),
				),
			},
			{
				Config: undent.String(`
				data "katapult_load_balancers" "src" {
					include_rules = true
				}
				`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.katapult_load_balancers.src",
						"load_balancers.#",
						"2",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_load_balancers.src",
						"load_balancers.0.rules.#",
						"1",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_load_balancers.src",
						"load_balancers.1.rules.0.destination_port",
						"8443",
					),
				),
			},
		},
	})
}
