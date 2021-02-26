package provider

import (
	"fmt"
	"log"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/pkg/katapult"
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

	var loadBalancers []*katapult.LoadBalancer
	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		pageResult, resp, err := m.Client.LoadBalancers.List(
			m.Ctx, m.OrganizationRef(), &katapult.ListOptions{Page: pageNum},
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

		log.Printf(
			"[DEBUG]  - Deleting Load Balancer %s (%s)\n", lb.ID, lb.Name,
		)
		_, _, err := m.Client.LoadBalancers.Delete(m.Ctx, lb)
		if err != nil {
			return err
		}
	}

	return nil
}

func TestAccKatapultLoadBalancer_basic(t *testing.T) {
	t.Skip("not yet feature complete")
	tt := NewTestTools(t)
	defer tt.Cleanup()

	name := tt.ResourceName("basic")
	res := "katapult_load_balancer.main"

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
					testAccCheckKatapultLoadBalancerExists(tt, res),
					resource.TestCheckResourceAttr(res, "name", name),
					resource.TestCheckResourceAttr(res,
						"resource_type",
						string(katapult.VirtualMachinesResourceType),
					),
				),
			},
			{
				ResourceName:      res,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultLoadBalancer_generated_name(t *testing.T) {
	t.Skip("not yet feature complete")
	tt := NewTestTools(t)
	defer tt.Cleanup()

	res := "katapult_load_balancer.main"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultLoadBalancerDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: `resource "katapult_load_balancer" "main" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerExists(tt, res),
					testCheckGeneratedResourceName(res, "name"),
					resource.TestCheckResourceAttr(res,
						"resource_type",
						string(katapult.VirtualMachinesResourceType),
					),
				),
			},
		},
	})
}

func TestAccKatapultLoadBalancer_update_name(t *testing.T) {
	t.Skip("not yet feature complete")
	tt := NewTestTools(t)
	defer tt.Cleanup()

	name := tt.ResourceName("update_name")
	res := "katapult_load_balancer.main"

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
					testAccCheckKatapultLoadBalancerExists(tt, res),
					resource.TestCheckResourceAttr(res, "name", name),
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
					testAccCheckKatapultLoadBalancerExists(tt, res),
					resource.TestCheckResourceAttr(res,
						"name", name+"-different",
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
	tt *TestTools,
	res string,
) resource.TestCheckFunc {
	m := tt.Meta

	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("resource not found: %s", res)
		}

		lb, _, err := m.Client.LoadBalancers.GetByID(
			tt.Meta.Ctx, rs.Primary.ID,
		)
		if err != nil {
			return err
		}

		return resource.TestCheckResourceAttr(res, "name", lb.Name)(s)
	}
}

//nolint:unused
func testAccCheckKatapultLoadBalancerDestroy(
	tt *TestTools,
) resource.TestCheckFunc {
	m := tt.Meta

	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "katapult_load_balancer" {
				continue
			}

			lb, _, err := m.Client.LoadBalancers.GetByID(m.Ctx, rs.Primary.ID)
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
