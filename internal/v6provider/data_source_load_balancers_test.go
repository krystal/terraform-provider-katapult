package v6provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceLoadBalancers_minimal(t *testing.T) {
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

					resource "katapult_load_balancer" "second" {
						name = "%s-t"
						depends_on = [katapult_load_balancer.first]
					}`,
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
