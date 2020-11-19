package provider

import (
	"fmt"
	"log"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/krystal/go-katapult/pkg/katapult"
)

func init() { //nolint:gochecknoinits
	resource.AddTestSweepers("katapult_load_balancer", &resource.Sweeper{
		Name: "katapult_load_balancer",
		F:    testSweepLoadBalancers,
	})
}

func testSweepLoadBalancers(_ string) error {
	m := sweepMeta()

	var testLbs []*katapult.LoadBalancer

	lbs, totalPages, err := testSweepLoadBalancersGetPage(m, 1)
	if err != nil {
		return err
	}
	testLbs = append(testLbs, lbs...)

	for page := 2; page <= totalPages; page++ {
		lbs, _, err := testSweepLoadBalancersGetPage(m, page)
		if err != nil {
			return err
		}
		testLbs = append(testLbs, lbs...)
	}

	for _, lb := range testLbs {
		log.Printf(
			"[DEBUG]  - Deleting load balancer %s (%s)\n", lb.Name, lb.ID,
		)
		_, _, err := m.Client.LoadBalancers.Delete(m.Ctx, lb.ID)
		if err != nil {
			return err
		}
	}

	return nil
}

func testSweepLoadBalancersGetPage(
	m *Meta,
	page int,
) ([]*katapult.LoadBalancer, int, error) {
	lbs, resp, err := m.Client.LoadBalancers.List(
		m.Ctx, m.OrganizationID, &katapult.ListOptions{Page: page},
	)
	if err != nil {
		return nil, 0, err
	}

	var testLbs []*katapult.LoadBalancer
	for _, lb := range lbs {
		if strings.HasPrefix(lb.Name, testAccResourceNamePrefix) {
			testLbs = append(testLbs, lb)
		}
	}

	totalPages := 1
	if resp != nil && resp.Pagination.TotalPages != 0 {
		totalPages = resp.Pagination.TotalPages
	}

	return testLbs, totalPages, nil
}

func TestAccKatapultLoadBalancer_basic(t *testing.T) {
	tt := NewTestTools(t)
	defer tt.Cleanup()

	name := tt.ResourceName("basic")
	res := "katapult_load_balancer.main"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: dedentf(`
					resource "katapult_load_balancer" "main" {
					  name = "%s"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccKatapultCheckLoadBalancerExists(tt, res),
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
	tt := NewTestTools(t)
	defer tt.Cleanup()

	res := "katapult_load_balancer.main"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `resource "katapult_load_balancer" "main" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccKatapultCheckLoadBalancerExists(tt, res),
					testCheckGeneratedResourceName(res),
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
	tt := NewTestTools(t)
	defer tt.Cleanup()

	name := tt.ResourceName("update_name")
	res := "katapult_load_balancer.main"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: dedentf(`
					resource "katapult_load_balancer" "main" {
					  name = "%s"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccKatapultCheckLoadBalancerExists(tt, res),
					resource.TestCheckResourceAttr(res, "name", name),
				),
			},
			{
				Config: dedentf(`
					resource "katapult_load_balancer" "main" {
					  name = "%s"
					}`,
					name+"-different",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccKatapultCheckLoadBalancerExists(tt, res),
					resource.TestCheckResourceAttr(res,
						"name", name+"-different",
					),
				),
			},
		},
	})
}

func testAccKatapultCheckLoadBalancerExists(
	tt *TestTools,
	res string,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := tt.Meta.Client

		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("resource not found: %s", res)
		}

		obj, _, err := c.LoadBalancers.Get(tt.Meta.Ctx, rs.Primary.ID)
		if err != nil {
			return err
		}

		if rs.Primary.Attributes["name"] != obj.Name {
			return fmt.Errorf(
				"expected name to be \"%s\", got \"%s\"",
				obj.Name, rs.Primary.Attributes["name"],
			)
		}

		return nil
	}
}
