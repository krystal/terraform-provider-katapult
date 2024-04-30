package v6provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/core"
)

func TestAccKatapultDataSourceLoadBalancer_basic(t *testing.T) {
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
						string(core.VirtualMachinesResourceType),
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceLoadBalancer_rules(t *testing.T) {
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
						"data.katapult_load_balancer.src",
						"name",
						name,
					),
					resource.TestCheckResourceAttr(
						"data.katapult_load_balancer.src",
						"resource_type",
						string(core.VirtualMachinesResourceType),
					),
				),
			},
		},
	})
}
