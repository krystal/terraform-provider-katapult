package v6provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/core"
)

func init() { //nolint:gochecknoinits
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

// Test with minimal required configuration.
func TestAccKatapultLoadBalancer_minimal(t *testing.T) {
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
					testAccCheckKatapultLoadBalancerAttrs(
						tt, "katapult_load_balancer.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main", "name", name,
					),
					// Verify default vaules of non-required fields.
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"virtual_machine_ids.#",
						"0",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"virtual_machine_group_ids.#",
						"0",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"tag_ids.#",
						"0",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"https_redirect", "false",
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

// Test with minimal required configuration.
func TestAccKatapultLoadBalancer_multi(t *testing.T) {
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
						name = "%s-main"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerAttrs(
						tt, "katapult_load_balancer.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main", "name", name+"-main",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "main" {
						name = "%s-main"
					}

					resource "katapult_load_balancer" "other" {
						name = "%s-other"
					}`,
					name, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerAttrs(
						tt, "katapult_load_balancer.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main", "name", name+"-main",
					),
					testAccCheckKatapultLoadBalancerAttrs(
						tt, "katapult_load_balancer.other",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.other", "name", name+"-other",
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

// Explicitly test all attributes with non-default values, and verify they can
// be modified.
func TestAccKatapultLoadBalancer_full(t *testing.T) {
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
						 https_redirect = true
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerAttrs(
						tt, "katapult_load_balancer.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main", "name", name,
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"https_redirect", "true",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "main" {
						name = "%s-foo"
						https_redirect = false
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerAttrs(
						tt, "katapult_load_balancer.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main", "name", name+"-foo",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"https_redirect", "false",
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

func TestAccKatapultLoadBalancer_vm(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultLoadBalancerDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_ip" "web" {}

					resource "katapult_virtual_machine" "base" {
						package       = "rock-3"
						disk_template = "ubuntu-22-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_ip.web.id]
					}

					resource "katapult_load_balancer" "main" {
						name = "%s"
						virtual_machine_ids = [katapult_virtual_machine.base.id]
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerAttrs(
						tt, "katapult_load_balancer.main",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_load_balancer.main",
						"virtual_machine_ids.*",
						"katapult_virtual_machine.base",
						"id",
					),
				),
			},
		},
	})
}

func TestAccKatapultLoadBalancer_vm_group(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultLoadBalancerDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_ip" "web" {}

					resource "katapult_virtual_machine_group" "web" {
						name = "web"
					}

					resource "katapult_virtual_machine" "base" {
						package       = "rock-3"
						disk_template = "ubuntu-22-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_ip.web.id]
						group_id = katapult_virtual_machine_group.web.id
					}

					resource "katapult_load_balancer" "main" {
						name = "%s"
						virtual_machine_group_ids = [
							katapult_virtual_machine_group.web.id
						]
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerAttrs(
						tt, "katapult_load_balancer.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"virtual_machine_group_ids.#",
						"1",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_load_balancer.main",
						"virtual_machine_group_ids.*",
						"katapult_virtual_machine_group.web",
						"id",
					),
				),
			},
		},
	})
}

func TestAccKatapultLoadBalancer_tag(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultLoadBalancerDestroy(tt),
		Steps: []resource.TestStep{
			{
				// TODO: Update hard-coded tag ID when katapult_tag resource is
				// implemented.
				Config: undent.Stringf(`
					resource "katapult_ip" "web" {}

					resource "katapult_virtual_machine" "base" {
						package       = "rock-3"
						disk_template = "ubuntu-22-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_ip.web.id]
						tags = ["web"]
					}

					resource "katapult_load_balancer" "main" {
						name = "%s"
						tag_ids = ["tag_NqAjIfOyzSMyuFPS"]
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerAttrs(
						tt, "katapult_load_balancer.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"tag_ids.#",
						"1",
					),
					resource.TestCheckTypeSetElemAttr(
						"katapult_load_balancer.main",
						"tag_ids.*",
						"tag_NqAjIfOyzSMyuFPS",
					),
				),
			},
		},
	})
}

func TestAccKatapultLoadBalancer_resource_type_change(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultLoadBalancerDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_ip" "web" {}

					resource "katapult_virtual_machine_group" "web" {
						name = "web"
					}

					resource "katapult_virtual_machine" "base" {
						package       = "rock-3"
						disk_template = "ubuntu-22-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_ip.web.id]
						tags = ["web"]
					}

					resource "katapult_load_balancer" "main" {
						name = "%s"
						virtual_machine_ids = [katapult_virtual_machine.base.id]
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerAttrs(
						tt, "katapult_load_balancer.main",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_load_balancer.main",
						"virtual_machine_ids.*",
						"katapult_virtual_machine.base",
						"id",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"virtual_machine_ids.#",
						"1",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"virtual_machine_group_ids.#",
						"0",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"tag_ids.#",
						"0",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_ip" "web" {}

					resource "katapult_virtual_machine_group" "web" {
						name = "web"
					}

					resource "katapult_virtual_machine" "base" {
						package       = "rock-3"
						disk_template = "ubuntu-22-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_ip.web.id]
						tags = ["web"]
					}

					resource "katapult_load_balancer" "main" {
						name = "%s"
						virtual_machine_group_ids = [
							katapult_virtual_machine_group.web.id
						]
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerAttrs(
						tt, "katapult_load_balancer.main",
					),
					resource.TestCheckTypeSetElemAttrPair(
						"katapult_load_balancer.main",
						"virtual_machine_group_ids.*",
						"katapult_virtual_machine_group.web",
						"id",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"virtual_machine_ids.#",
						"0",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"virtual_machine_group_ids.#",
						"1",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"tag_ids.#",
						"0",
					),
				),
			},
			{
				// TODO: Update hard-coded tag ID when katapult_tag resource is
				// implemented.
				Config: undent.Stringf(`
					resource "katapult_ip" "web" {}

					resource "katapult_virtual_machine_group" "web" {
						name = "web"
					}

					resource "katapult_virtual_machine" "base" {
						package       = "rock-3"
						disk_template = "ubuntu-22-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_ip.web.id]
						tags = ["web"]
					}

					resource "katapult_load_balancer" "main" {
						name = "%s"
						tag_ids = ["tag_NqAjIfOyzSMyuFPS"]
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerAttrs(
						tt, "katapult_load_balancer.main",
					),
					resource.TestCheckTypeSetElemAttr(
						"katapult_load_balancer.main",
						"tag_ids.*",
						"tag_NqAjIfOyzSMyuFPS",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"virtual_machine_ids.#",
						"0",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"virtual_machine_group_ids.#",
						"0",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"tag_ids.#",
						"1",
					),
				),
			},
		},
	})
}

func TestAccKatapultLoadBalancer_vms_update(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultLoadBalancerDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_ip" "base" {}

					resource "katapult_virtual_machine" "base" {
						package       = "rock-3"
						disk_template = "ubuntu-22-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_ip.base.id]
					}

					resource "katapult_load_balancer" "main" {
						name = "%s"
						virtual_machine_ids = [
							katapult_virtual_machine.base.id,
						]
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerAttrs(
						tt, "katapult_load_balancer.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"virtual_machine_ids.#",
						"1",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_ip" "base" {}

					resource "katapult_virtual_machine" "base" {
						package       = "rock-3"
						disk_template = "ubuntu-22-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_ip.base.id]
					}

					resource "katapult_ip" "web" {}

					resource "katapult_virtual_machine" "web" {
						package       = "rock-3"
						disk_template = "ubuntu-22-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_ip.web.id]
					}

					resource "katapult_load_balancer" "main" {
						name = "%s"
						virtual_machine_ids = [
							katapult_virtual_machine.web.id,
							katapult_virtual_machine.base.id
						]
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerAttrs(
						tt, "katapult_load_balancer.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"virtual_machine_ids.#",
						"2",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_ip" "base" {}

					resource "katapult_virtual_machine" "base" {
						package       = "rock-3"
						disk_template = "ubuntu-22-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_ip.base.id]
					}

					resource "katapult_ip" "web" {}

					resource "katapult_virtual_machine" "web" {
						package       = "rock-3"
						disk_template = "ubuntu-22-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_ip.web.id]
					}

					resource "katapult_load_balancer" "main" {
						name = "%s"
						virtual_machine_ids = [
							katapult_virtual_machine.base.id,
							katapult_virtual_machine.web.id
						]
					}`,
					name,
				),
				// We want to assert that the plan is empty, as the order of the
				// virtual_machine_ids should not matter.
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerAttrs(
						tt, "katapult_load_balancer.main",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer.main",
						"virtual_machine_ids.#",
						"2",
					),
				),
			},
		},
	})
}

// Test that the ID attribute does not get marked as unknown when the resource
// is modified, which in turn would case associated rules to be re-created.
func TestAccKatapultLoadBalancer_stableID(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	var oldRuleID, currentRuleID string

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultLoadBalancerDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "main" {
						name = "%s"
						https_redirect = false
					}

					resource "katapult_load_balancer_rule" "https" {
						load_balancer_id = katapult_load_balancer.main.id
						destination_port = 8443
						listen_port = 443
						protocol = "HTTPS"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerAttrs(
						tt, "katapult_load_balancer.main",
					),
					testAccCheckKatapultLoadBalancerRuleExists(
						tt, "katapult_load_balancer_rule.https", &oldRuleID,
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "main" {
						name = "%s"
						https_redirect = true
					}

					resource "katapult_load_balancer_rule" "https" {
						load_balancer_id = katapult_load_balancer.main.id
						destination_port = 8443
						listen_port = 443
						protocol = "HTTPS"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerAttrs(
						tt, "katapult_load_balancer.main",
					),
					testAccCheckKatapultLoadBalancerRuleExists(
						tt, "katapult_load_balancer_rule.https",
						&currentRuleID,
					),
					testAccCheckResourceAttrNotChanged(
						"katapult_load_balancer_rule.https", "id",
						&oldRuleID, &currentRuleID,
					),
				),
			},
		},
	})
}

//
// Helpers
//

func testAccCheckKatapultLoadBalancerAttrs(
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

		var resourceAttribute string
		var otherResourceAttrs []string
		switch lb.ResourceType {
		case core.VirtualMachinesResourceType:
			resourceAttribute = "virtual_machine_ids"
			otherResourceAttrs = []string{
				"virtual_machine_group_ids",
				"tag_ids",
			}
		case core.VirtualMachineGroupsResourceType:
			resourceAttribute = "virtual_machine_group_ids"
			otherResourceAttrs = []string{
				"virtual_machine_ids",
				"tag_ids",
			}
		case core.TagsResourceType:
			resourceAttribute = "tag_ids"
			otherResourceAttrs = []string{
				"virtual_machine_ids",
				"virtual_machine_group_ids",
			}
		}

		tfs := []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(res, "id", lb.ID),
			resource.TestCheckResourceAttr(res, "name", lb.Name),
			resource.TestCheckResourceAttr(
				res, "ip_address", lb.IPAddress.Address,
			),
			resource.TestCheckResourceAttr(
				res, "https_redirect", strconv.FormatBool(lb.HTTPSRedirect),
			),
			resource.TestCheckResourceAttr(
				res, resourceAttribute+".#", strconv.Itoa(len(lb.ResourceIDs)),
			),
		}

		for _, attr := range otherResourceAttrs {
			tfs = append(tfs,
				resource.TestCheckResourceAttr(
					res, attr+".#", "0",
				),
			)
		}

		for _, id := range lb.ResourceIDs {
			tfs = append(tfs,
				resource.TestCheckTypeSetElemAttr(
					res, resourceAttribute+".*", id,
				),
			)
		}

		return resource.ComposeAggregateTestCheckFunc(tfs...)(s)
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
