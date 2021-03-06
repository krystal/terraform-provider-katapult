package provider

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/core"
)

func init() { //nolint:gochecknoinits
	// TODO: re-enable katapult_load_balancer sweeper when Load Balancer
	// resources are enabled again.
	// resource.AddTestSweepers("katapult_load_balancer", &resource.Sweeper{
	//		Name: "katapult_load_balancer",
	//		F:    testSweepLoadBalancers,
	// })
}

//nolint:deadcode,unused
func testSweepLoadBalancers(_ string) error {
	m := sweepMeta()
	ctx := context.TODO()

	var loadBalancers []*core.LoadBalancer
	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		pageResult, resp, err := m.Core.LoadBalancers.List(
			ctx, m.OrganizationRef, &core.ListOptions{Page: pageNum},
		)
		if err != nil {
			return err
		}

		totalPages = resp.Pagination.TotalPages
		loadBalancers = append(loadBalancers, pageResult...)
	}

	for _, lb := range loadBalancers {
		if !strings.HasPrefix(lb.Name, testAccResourceNamePrefix) {
			continue
		}

		m.Logger.Info("deleting load balancer", "id", lb.ID, "name", lb.Name)
		_, _, err := m.Core.LoadBalancers.Delete(ctx, lb.Ref())
		if err != nil {
			return err
		}
	}

	return nil
}

func TestAccKatapultLoadBalancer_basic(t *testing.T) {
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
						string(core.VirtualMachinesResourceType),
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
	t.Skip("not yet feature complete")
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultLoadBalancerDestroy(tt),
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
						string(core.VirtualMachinesResourceType),
					),
				),
			},
		},
	})
}

func TestAccKatapultLoadBalancer_update_name(t *testing.T) {
	t.Skip("not yet feature complete")
	tt := newTestTools(t)

	name := tt.ResourceName("update_name")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultLoadBalancerDestroy(tt),
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

//nolint:unused
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

		lb, _, err := m.Core.LoadBalancers.GetByID(
			tt.Ctx, rs.Primary.ID,
		)
		if err != nil {
			return err
		}

		return resource.TestCheckResourceAttr(res, "name", lb.Name)(s)
	}
}

//nolint:unused
func testAccCheckKatapultLoadBalancerDestroy(
	tt *testTools,
) resource.TestCheckFunc {
	m := tt.Meta

	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "katapult_load_balancer" {
				continue
			}

			lb, _, err := m.Core.LoadBalancers.GetByID(tt.Ctx, rs.Primary.ID)
			if err == nil && lb != nil {
				return fmt.Errorf(
					"katapult_load_balancer %s (%s) was not destroyed",
					rs.Primary.ID, lb.Name,
				)
			}
		}

		return nil
	}
}
