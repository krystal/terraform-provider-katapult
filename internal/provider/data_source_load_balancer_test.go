package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/pkg/katapult"
)

func TestAccKatapultDataSourceLoadBalancer_basic(t *testing.T) {
	t.Skip("not yet feature complete")
	tt := newTestTools(t)

	name := tt.ResourceName("basic")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultLoadBalancerDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "main" {
					  name = "%s"
					}

					data "katapult_load_balancer" "src" {
					  id = katapult_load_balancer.main.id
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerExists(
						tt, "data.katapult_load_balancer.src",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_load_balancer.src", "name", name,
					),
					resource.TestCheckResourceAttr(
						"data.katapult_load_balancer.src", "resource_type",
						string(katapult.VirtualMachinesResourceType),
					),
				),
			},
		},
	})
}
