package v6provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceLoadBalancerRule_minimal(t *testing.T) {
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
					}

					data "katapult_load_balancer_rule" "src" {
						id = katapult_load_balancer_rule.my_rule.id
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerRuleAttrs(
						tt, "data.katapult_load_balancer_rule.src",
					),
				),
			},
		},
	})
}
