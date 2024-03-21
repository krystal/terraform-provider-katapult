package v6provider

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/core"
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

func TestAccKatapultLoadBalancer_basicWithExternal(t *testing.T) {
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
					  external_rules = true
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
				ImportStateVerify: false,
				ImportStateCheck: func(s []*terraform.InstanceState) error {
					if len(s) != 1 {
						return fmt.Errorf(
							"expected 1 instance state, got %d", len(s),
						)
					}

					if s[0].ID == "" {
						return fmt.Errorf("instance state ID is empty")
					}

					if s[0].Attributes["name"] != name {
						return fmt.Errorf(
							"instance state name is %q, expected %q",
							s[0].Attributes["name"], name,
						)
					}

					return nil
				},
			},
		},
	})
}

func TestAccKatapultLoadBalancer_basicWithRule(t *testing.T) {
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
					  rules = [
						{
							destination_port = 8080
							listen_port = 80
							protocol = "HTTP"
							passthrough_ssl = false
						}
					  ]
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
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"rules.0.destination_port",
						"8080",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"rules.0.listen_port",
						"80",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"rules.0.protocol",
						"HTTP",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"rules.0.passthrough_ssl",
						"false",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"rules.0.check_enabled",
						"false",
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

func TestAccKatapultLoadBalancer_basicWithNoRules(t *testing.T) {
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
					  rules = []
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

func TestAccKatapultLoadBalancer_basicWithNullRules(t *testing.T) {
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

func TestAccKatapultLoadBalancer_basicWithRules(t *testing.T) {
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
					  rules = [
						{
							destination_port = 8080
							listen_port = 80
							protocol = "HTTP"
							passthrough_ssl = false
						},
						{
							destination_port = 8443
							listen_port = 443
							protocol = "HTTPS"
							passthrough_ssl = true
						}
					  ]
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
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"rules.0.destination_port",
						"8080",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"rules.0.listen_port",
						"80",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"rules.0.protocol",
						"HTTP",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"rules.0.passthrough_ssl",
						"false",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"rules.0.check_enabled",
						"false",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"rules.1.destination_port",
						"8443",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"rules.1.listen_port",
						"443",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"rules.1.protocol",
						"HTTPS",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"rules.1.passthrough_ssl",
						"true",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"rules.1.check_enabled",
						"false",
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
				Config: `resource "katapult_load_balancer" "main" {
					external_rules = true
				}`,
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
					  external_rules = true
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
					  external_rules = true
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

		lb, _, err := m.Core.LoadBalancers.GetByID(
			tt.Ctx, rs.Primary.ID,
		)
		if err != nil {
			return err
		}

		return resource.TestCheckResourceAttr(res, "name", lb.Name)(s)
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
