package v6provider

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/next/core"
)

func init() { //nolint:gochecknoinits
	// TODO: re-enable katapult_load_balancer sweeper when Load Balancer
	// resources are enabled again.
	resource.AddTestSweepers("katapult_load_balancer", &resource.Sweeper{
		Name: "katapult_load_balancer",
		F:    testSweepLoadBalancers,
	})
}

func testSweepLoadBalancers(_ string) error {
	m := sweepMeta()
	ctx := context.TODO()

	//nolint:lll // type is generated
	var loadBalancers []core.GetOrganizationLoadBalancers200ResponseLoadBalancers
	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		// pageResult, resp, err := m.Core.LoadBalancers.List(
		// 	ctx, m.OrganizationRef, &core.ListOptions{Page: pageNum},
		// )
		// if err != nil {
		// 	return err
		// }

		res, err := m.Core.GetOrganizationLoadBalancersWithResponse(ctx,
			&core.GetOrganizationLoadBalancersParams{
				OrganizationId: &m.confOrganization,
				Page:           &pageNum,
			})
		if err != nil {
			return err
		}

		resp := res.JSON200

		totalPages = *resp.Pagination.TotalPages
		loadBalancers = append(loadBalancers, resp.LoadBalancers...)
	}

	for _, lb := range loadBalancers {
		if !strings.HasPrefix(*lb.Name, testAccResourceNamePrefix) {
			continue
		}

		m.Logger.Info("deleting load balancer", "id", lb.Id, "name", lb.Name)
		_, err := m.Core.DeleteLoadBalancerWithResponse(ctx,
			core.DeleteLoadBalancerJSONRequestBody{
				LoadBalancer: core.LoadBalancerLookup{
					Id: lb.Id,
				},
			})
		if err != nil {
			return err
		}
	}

	return nil
}

func TestAccKatapultLoadBalancer_basic(t *testing.T) {
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
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerExists(
						tt, "katapult_load_balancer.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main", "name", name,
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"resource_type",
						string(core.VirtualMachines),
					),
				),
			},
			{
				ResourceName:      "katapult_load_balancer.main",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultLoadBalancer_generated_name(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultLoadBalancerDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: `resource "katapult_load_balancer" "main" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerExists(
						tt, "katapult_load_balancer.main",
					),
					testCheckGeneratedResourceName(
						"katapult_load_balancer.main", "name",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"resource_type",
						string(core.VirtualMachines),
					),
				),
			},
		},
	})
}

func TestAccKatapultLoadBalancer_update_name(t *testing.T) {
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
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerExists(
						tt, "katapult_load_balancer.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main", "name", name,
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "main" {
					  name = "%s"
					}`,
					name+"-different",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerExists(
						tt, "katapult_load_balancer.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main", "name",
						name+"-different",
					),
				),
			},
		},
	})
}

//
// Helpers
//

func testAccCheckKatapultLoadBalancerExists(
	tt *testTools,
	res string,
) resource.TestCheckFunc {
	m := tt.Meta

	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("resource not found: %s", res)
		}

		resp, err := m.Core.GetLoadBalancerWithResponse(tt.Ctx,
			&core.GetLoadBalancerParams{
				LoadBalancerId: &rs.Primary.ID,
			})
		if err != nil {
			return err
		}

		lb := resp.JSON200.LoadBalancer

		return resource.TestCheckResourceAttr(res, "name", *lb.Name)(s)
	}
}

func testAccCheckKatapultLoadBalancerDestroy(
	tt *testTools,
) resource.TestCheckFunc {
	m := tt.Meta

	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "katapult_load_balancer" {
				continue
			}

			// lb, _, err := m.Core.LoadBalancers.GetByID(tt.Ctx, rs.Primary.ID)
			resp, err := m.Core.GetLoadBalancerWithResponse(tt.Ctx,
				&core.GetLoadBalancerParams{
					LoadBalancerId: &rs.Primary.ID,
				})

			if err == nil && resp.JSON200 != nil {
				return fmt.Errorf(
					"katapult_load_balancer %s (%s) was not destroyed",
					rs.Primary.ID, *resp.JSON200.LoadBalancer.Name,
				)
			}
		}

		return nil
	}
}
