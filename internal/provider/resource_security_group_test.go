package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/dnaeon/go-vcr/recorder"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/core"
	"github.com/stretchr/testify/assert"
)

//
// Terraform Operations
//

func init() { //nolint:gochecknoinits
	resource.AddTestSweepers("katapult_security_group", &resource.Sweeper{
		Name: "katapult_security_group",
		F:    testSweepSecurityGroups,
	})
}

func testSweepSecurityGroups(_ string) error {
	m := sweepMeta()
	ctx := context.Background()

	toDelete := []*core.SecurityGroup{}
	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		pageResults, resp, err := m.Core.SecurityGroups.List(
			ctx, m.OrganizationRef, &core.ListOptions{Page: pageNum},
		)
		if err != nil {
			return err
		}

		totalPages = resp.Pagination.TotalPages
		for _, sg := range pageResults {
			if strings.HasPrefix(sg.Name, testAccResourceNamePrefix) {
				toDelete = append(toDelete, sg)
			}
		}
	}

	for _, sg := range toDelete {
		m.Logger.Info(
			"deleting security group", "id", sg.ID, "name", sg.Name,
		)
		_, _, err := m.Core.SecurityGroups.Delete(ctx, sg.Ref())
		if err != nil {
			return err
		}
	}

	return nil
}

//
// Tests
//

func TestAccKatapultSecurityGroup_example(t *testing.T) {
	if vcrMode() == recorder.ModeReplaying {
		t.Skip("example based tests are not supported in replay mode")
	}

	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultSecurityGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: exampleResourceConfig(
					t, "katapult_security_group",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "katapult_security_group.minimal",
					),
					testAccCheckKatapultSecurityGroupExists(
						tt, "katapult_security_group.practical",
					),
					testAccCheckKatapultSecurityGroupExists(
						tt, "katapult_security_group.dynamic",
					),
				),
			},
		},
	})
}

func TestAccKatapultSecurityGroup_minimal(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultSecurityGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "katapult_security_group.my_sg",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"associations.#", "0",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"allow_all_inbound", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"allow_all_outbound", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rule.#", "0",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"outbound_rule.#", "0",
					),
				),
			},
			{
				ResourceName:      "katapult_security_group.my_sg",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultSecurityGroup_allow_all_inbound(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultSecurityGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"
						allow_all_inbound = true
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "katapult_security_group.my_sg",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"associations.#", "0",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"allow_all_outbound", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rule.#", "0",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"outbound_rule.#", "0",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"
						allow_all_inbound = false
						allow_all_outbound = true
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "katapult_security_group.my_sg",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"associations.#", "0",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rule.#", "0",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"outbound_rule.#", "0",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "katapult_security_group.my_sg",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"associations.#", "0",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"allow_all_inbound", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"allow_all_outbound", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rule.#", "0",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"outbound_rule.#", "0",
					),
				),
			},
			{
				ResourceName:      "katapult_security_group.my_sg",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultSecurityGroup_allow_all_outbound(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultSecurityGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"
						allow_all_outbound = true
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "katapult_security_group.my_sg",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"associations.#", "0",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"allow_all_inbound", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rule.#", "0",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"outbound_rule.#", "0",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"
						allow_all_inbound = true
						allow_all_outbound = false
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "katapult_security_group.my_sg",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"associations.#", "0",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rule.#", "0",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"outbound_rule.#", "0",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "katapult_security_group.my_sg",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"associations.#", "0",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"allow_all_inbound", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"allow_all_outbound", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rule.#", "0",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"outbound_rule.#", "0",
					),
				),
			},
			{
				ResourceName:      "katapult_security_group.my_sg",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultSecurityGroup_associations(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultSecurityGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_virtual_machine_group" "web" {
						name = "%s"
					}

					resource "katapult_security_group" "my_sg" {
						name = "%s"
						associations = [katapult_virtual_machine_group.web.id]
					}`,
					name, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "katapult_security_group.my_sg",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"allow_all_inbound", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"allow_all_outbound", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rule.#", "0",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"outbound_rule.#", "0",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_virtual_machine_group" "web" {
						name = "%s"
					}

					resource "katapult_virtual_machine_group" "db" {
						name = "%s-db"
					}

					resource "katapult_security_group" "my_sg" {
						name = "%s"
						associations = [
							katapult_virtual_machine_group.web.id,
							katapult_virtual_machine_group.db.id,
						]
					}`,
					name, name, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "katapult_security_group.my_sg",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"allow_all_inbound", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"allow_all_outbound", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rule.#", "0",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"outbound_rule.#", "0",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"
						associations = []
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "katapult_security_group.my_sg",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"allow_all_inbound", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"allow_all_outbound", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rule.#", "0",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"outbound_rule.#", "0",
					),
				),
			},
			{
				ResourceName:      "katapult_security_group.my_sg",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultSecurityGroup_rules(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultSecurityGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"
						allow_all_inbound = true
						allow_all_outbound = true
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "katapult_security_group.my_sg",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"associations.#", "0",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rule.#", "0",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"outbound_rule.#", "0",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_virtual_machine_group" "web" {
						name = "%s"
					}

					resource "katapult_security_group" "my_sg" {
						name = "%s"
						associations = [
							katapult_virtual_machine_group.web.id,
						]
						allow_all_inbound = false
						allow_all_outbound = true

						inbound_rule {
							protocol = "tcp"
							ports = "22"
							targets = ["10.0.0.0/8"]
							notes = "SSH"
						}
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "katapult_security_group.my_sg",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rule.0.direction", "inbound",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"outbound_rule.#", "0",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_virtual_machine_group" "web" {
						name = "%s"
					}

					resource "katapult_virtual_machine_group" "monitoring" {
						name = "%s"
					}

					resource "katapult_security_group" "my_sg" {
						name = "%s"
						associations = [
							katapult_virtual_machine_group.web.id,
						]
						allow_all_inbound = false
						allow_all_outbound = true

						inbound_rule {
							protocol = "tcp"
							ports = "22"
							targets = ["10.0.0.0/8"]
							notes = "SSH"
						}
						inbound_rule {
							protocol = "tcp"
							ports = "80"
							targets = ["all:ipv4", "all:ipv6"]
							notes = "HTTP"
						}
						inbound_rule {
							protocol = "TCP"
							ports = "443"
							targets = ["all:ipv4", "all:ipv6"]
							notes = "HTTPS"
						}
						inbound_rule {
							protocol = "icmp"
							targets = [
								katapult_virtual_machine_group.monitoring.id
							]
							notes = "ping"
						}
						inbound_rule {
							protocol = "udp"
							ports = "443"
							targets = ["all:ipv4", "all:ipv6"]
							notes = "QUIC"
						}
					}`,
					name, name, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "katapult_security_group.my_sg",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rule.0.direction", "inbound",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rule.1.direction", "inbound",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rule.2.direction", "inbound",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rule.3.direction", "inbound",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rule.4.direction", "inbound",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"outbound_rule.#", "0",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_virtual_machine_group" "web" {
						name = "%s"
					}

					resource "katapult_virtual_machine_group" "db" {
						name = "%s"
					}

					resource "katapult_security_group" "my_sg" {
						name = "%s"
						associations = [
							katapult_virtual_machine_group.web.id,
						]
						allow_all_inbound = false
						allow_all_outbound = false

						inbound_rule {
							protocol = "tcp"
							ports = "22"
							targets = ["10.0.0.0/8"]
							notes = "SSH"
						}
						inbound_rule {
							protocol = "tcp"
							ports = "80"
							targets = ["all:ipv4", "all:ipv6"]
							notes = "HTTP"
						}
						inbound_rule {
							protocol = "TCP"
							ports = "443"
							targets = ["all:ipv4", "all:ipv6"]
							notes = "HTTPS"
						}
						inbound_rule {
							protocol = "udp"
							ports = "443"
							targets = ["all:ipv4", "all:ipv6"]
							notes = "QUIC"
						}

						outbound_rule {
							protocol = "tcp"
							ports = "3306"
							targets = [katapult_virtual_machine_group.db.id]
							notes = "MySQL"
						}
						outbound_rule {
							protocol = "tcp"
							ports = "80,443"
							targets = ["all:ipv4", "all:ipv6"]
							notes = "HTTP & HTTPS"
						}
					}`,
					name, name, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "katapult_security_group.my_sg",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rule.0.direction", "inbound",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rule.1.direction", "inbound",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rule.2.direction", "inbound",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rule.3.direction", "inbound",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"outbound_rule.0.direction", "outbound",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"outbound_rule.1.direction", "outbound",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_virtual_machine_group" "web" {
						name = "%s"
					}

					resource "katapult_security_group" "my_sg" {
						name = "%s"
						associations = [
							katapult_virtual_machine_group.web.id,
						]
						allow_all_inbound = false
						allow_all_outbound = false

						inbound_rule {
							protocol = "tcp"
							ports = "22"
							targets = ["10.0.0.0/8"]
							notes = "SSH"
						}
						inbound_rule {
							protocol = "tcp"
							ports = "80"
							targets = ["all:ipv4", "all:ipv6"]
							notes = "HTTP"
						}
						inbound_rule {
							protocol = "TCP"
							ports = "443"
							targets = ["all:ipv4", "all:ipv6"]
							notes = "HTTPS"
						}
						inbound_rule {
							protocol = "udp"
							ports = "443"
							targets = ["all:ipv4", "all:ipv6"]
							notes = "QUIC"
						}

						outbound_rule {
							protocol = "tcp"
							ports = "80,443"
							targets = ["all:ipv4", "all:ipv6"]
							notes = "HTTP & HTTPS"
						}
					}`,
					name, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "katapult_security_group.my_sg",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rule.0.direction", "inbound",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rule.1.direction", "inbound",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rule.2.direction", "inbound",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rule.3.direction", "inbound",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"outbound_rule.0.direction", "outbound",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_virtual_machine_group" "web" {
						name = "%s"
					}

					resource "katapult_security_group" "my_sg" {
						name = "%s"
						associations = [
							katapult_virtual_machine_group.web.id,
						]
						allow_all_inbound = true
						allow_all_outbound = true
					}`,
					name, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "katapult_security_group.my_sg",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rule.#", "0",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"outbound_rule.#", "0",
					),
				),
			},
			{
				ResourceName:      "katapult_security_group.my_sg",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultSecurityGroup_dynamic_rules(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultSecurityGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
				//nolint:lll
				Config: undent.Stringf(`
					locals {
						my_rules = {
							inbound = [
								{
									protocol = "TCP"
									ports    = "22"
									targets  = ["all:ipv4", "all:ipv6"]
									notes    = "SSH"
								},
								{
									protocol = "TCP"
									ports    = "80,433"
									targets  = ["all:ipv4", "all:ipv6"]
									notes    = "HTTP & HTTPS"
								},
								{
									protocol = "UDP"
									ports    = "443"
									targets  = ["all:ipv4", "all:ipv6"]
									notes    = "QUIC"
								},
							]
							outbound = []
						}
					}

					resource "katapult_security_group" "my_sg" {
						name               = "%s"
						allow_all_inbound  = length(local.my_rules.inbound) > 0 ? false : true
						allow_all_outbound = length(local.my_rules.outbound) > 0 ? false : true

						dynamic "inbound_rule" {
							for_each = local.my_rules.inbound
							content {
								protocol = inbound_rule.value.protocol
								ports    = inbound_rule.value.ports
								targets  = inbound_rule.value.targets
								notes    = inbound_rule.value.notes
							}
						}

						dynamic "outbound_rule" {
							for_each = local.my_rules.outbound
							content {
								protocol = outbound_rule.value.protocol
								ports    = outbound_rule.value.ports
								targets  = outbound_rule.value.targets
								notes    = outbound_rule.value.notes
							}
						}
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "katapult_security_group.my_sg",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"associations.#", "0",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"allow_all_inbound", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"allow_all_outbound", "true",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"inbound_rule.#", "3",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg",
						"outbound_rule.#", "0",
					),
				),
			},
			{
				ResourceName:      "katapult_security_group.my_sg",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultSecurityGroup_invalid_rules(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultSecurityGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"
						allow_all_inbound = true

						inbound_rule {
							protocol = "tcp"
							ports = "22"
							targets = ["all:ipv4", "all:ipv6"]
							notes = "SSH"
						}
					}`,
					name,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						"cannot enable allow_all_inbound while also " +
							"specifyng one or more inbound_rule",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"
						allow_all_outbound = true

						outbound_rule {
							protocol = "tcp"
							ports = "22"
							targets = ["all:ipv4", "all:ipv6"]
							notes = "SSH"
						}
					}`,
					name,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						"cannot enable allow_all_outbound while also " +
							"specifyng one or more outbound_rule",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"
						external_rules = true

						inbound_rule {
							protocol = "tcp"
							ports = "22"
							targets = ["all:ipv4", "all:ipv6"]
							notes = "SSH"
						}
					}`,
					name,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						"\"external_rules\": conflicts with inbound_rule",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"
						external_rules = true

						outbound_rule {
							protocol = "tcp"
							ports = "22"
							targets = ["all:ipv4", "all:ipv6"]
							notes = "SSH"
						}
					}`,
					name,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						"\"external_rules\": conflicts with outbound_rule",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"

						inbound_rule {
							protocol = "grpc"
							ports = "443"
							targets = ["all:ipv4", "all:ipv6"]
							notes = "gRPC"
						}
					}`,
					name,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						"expected inbound_rule.0.protocol " +
							"to be one of [TCP UDP ICMP], got grpc",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"

						outbound_rule {
							protocol = "slashdot"
							ports = "443"
							targets = ["all:ipv4", "all:ipv6"]
							notes = "cool"
						}
					}`,
					name,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						"expected outbound_rule.0.protocol " +
							"to be one of [TCP UDP ICMP], got slashdot",
					),
				),
			},
		},
	})
}

func TestAccKatapultSecurityGroup_multiple(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultSecurityGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg_foo" {
						name = "%s-foo"

						inbound_rule {
							protocol = "tcp"
							ports = "22"
							targets = ["all:ipv4", "all:ipv6"]
							notes = "SSH"
						}
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "katapult_security_group.my_sg_foo",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg_foo",
						"associations.#", "0",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg_foo",
						"allow_all_inbound", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg_foo",
						"allow_all_outbound", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg_foo",
						"inbound_rule.#", "1",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg_foo",
						"outbound_rule.#", "0",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg_foo" {
						name = "%s-foo"

						inbound_rule {
							protocol = "tcp"
							ports = "22"
							targets = ["all:ipv4", "all:ipv6"]
							notes = "SSH"
						}
					}

					resource "katapult_security_group" "my_sg_bar" {
						name = "%s-bar"

						inbound_rule {
							protocol = "tcp"
							ports = "80"
							targets = ["all:ipv4", "all:ipv6"]
							notes = "HTTP"
						}
						inbound_rule {
							protocol = "tcp"
							ports = "433"
							targets = ["all:ipv4", "all:ipv6"]
							notes = "HTTPS"
						}
					}`,
					name, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "katapult_security_group.my_sg_foo",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg_foo",
						"associations.#", "0",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg_foo",
						"allow_all_inbound", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg_foo",
						"allow_all_outbound", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg_foo",
						"inbound_rule.#", "1",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg_foo",
						"outbound_rule.#", "0",
					),

					testAccCheckKatapultSecurityGroupExists(
						tt, "katapult_security_group.my_sg_bar",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg_bar",
						"associations.#", "0",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg_bar",
						"allow_all_inbound", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg_bar",
						"allow_all_outbound", "false",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg_bar",
						"inbound_rule.#", "2",
					),
					resource.TestCheckResourceAttr(
						"katapult_security_group.my_sg_bar",
						"outbound_rule.#", "0",
					),
				),
			},
			{
				ResourceName:      "katapult_security_group.my_sg_foo",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "katapult_security_group.my_sg_bar",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

//
// Test Helpers
//

func testAccCheckKatapultSecurityGroupDestroy(
	tt *testTools,
) resource.TestCheckFunc {
	m := tt.Meta

	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "katapult_security_group" {
				continue
			}

			sg, _, err := m.Core.SecurityGroups.GetByID(
				tt.Ctx, rs.Primary.ID,
			)

			if err == nil && sg != nil {
				return fmt.Errorf(
					"katapult_security_group %s (%s) was not destroyed",
					rs.Primary.ID, sg.Name,
				)
			}
		}

		return nil
	}
}

func testAccCheckKatapultSecurityGroupExists(
	tt *testTools,
	res string,
) resource.TestCheckFunc {
	m := tt.Meta

	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("resource not found: %s", res)
		}

		sg, _, err := m.Core.SecurityGroups.GetByID(
			tt.Ctx, rs.Primary.ID,
		)
		if err != nil {
			return err
		}

		return resource.TestCheckResourceAttr(res, "id", sg.ID)(s)
	}
}

//
// Helper Tests
//

var (
	sshRuleID = "sgr_rdSgXlEJ1sL6XmR4"
	sshRulev0 = map[string]any{
		"protocol": "tcp",
		"ports":    "22",
		"targets":  schema.NewSet(schema.HashString, []any{"all:ipv4"}),
		"notes":    "SSH",
	}
	sshRulev1 = map[string]any{
		"id":       sshRuleID,
		"protocol": "tcp",
		"ports":    "22",
		"targets":  schema.NewSet(schema.HashString, []any{"all:ipv4"}),
		"notes":    "SSH",
	}
	sshRulev2 = map[string]any{
		"id":       sshRuleID,
		"protocol": "tcp",
		"ports":    "22,722",
		"targets": schema.NewSet(
			schema.HashString,
			[]any{"all:ipv4", "all:ipv6"},
		),
		"notes": "SSH",
	}

	dnsRuleID = "sgr_sO2uWbkDeOYep5K4"
	dnsRulev0 = map[string]any{
		"protocol": "tcp",
		"ports":    "53",
		"targets": schema.NewSet(
			schema.HashString,
			[]any{"1.1.1.1", "1.0.0.1"},
		),
		"notes": "Cloudflare DNS",
	}
	dnsRulev1 = map[string]any{
		"id":       dnsRuleID,
		"protocol": "tcp",
		"ports":    "53",
		"targets": schema.NewSet(
			schema.HashString,
			[]any{"1.1.1.1", "1.0.0.1"},
		),
		"notes": "Cloudflare DNS",
	}
	dnsRulev2 = map[string]any{
		"id":       dnsRuleID,
		"protocol": "tcp",
		"ports":    "53",
		"targets": schema.NewSet(
			schema.HashString,
			[]any{"1.1.1.1", "1.0.0.1", "8.8.8.8", "8.8.4.4"},
		),
		"notes": "Public DNS",
	}

	httpRuleID = "sgr_v3lf3DOIJ3EDPCGg"
	httpRulev0 = map[string]any{
		"protocol": "tcp",
		"ports":    "80,443",
		"targets":  schema.NewSet(schema.HashString, []any{"all:ipv4"}),
		"notes":    "HTTP/HTTPS",
	}
	httpRulev1 = map[string]any{
		"id":       httpRuleID,
		"protocol": "tcp",
		"ports":    "80,443",
		"targets":  schema.NewSet(schema.HashString, []any{"all:ipv4"}),
		"notes":    "HTTP/HTTPS",
	}
	httpRulev2 = map[string]any{
		"id":       httpRuleID,
		"protocol": "tcp",
		"ports":    "80,443",
		"targets": schema.NewSet(
			schema.HashString,
			[]any{"all:ipv4", "all:ipv6"},
		),
		"notes": "HTTP & HTTPS",
	}
)

func Test_diffSecurityGroupRules(t *testing.T) {
	type args struct {
		oldRules []any
		newRules []any
	}
	tests := []struct {
		name       string
		args       args
		wantCreate []map[string]any
		wantUpdate []map[string]any
		wantDelete []map[string]any
	}{
		{
			name: "create first rule",
			args: args{
				oldRules: []any{},
				newRules: []any{sshRulev0},
			},
			wantCreate: []map[string]any{sshRulev0},
		},
		{
			name: "create many rules",
			args: args{
				oldRules: []any{},
				newRules: []any{sshRulev0, dnsRulev0},
			},
			wantCreate: []map[string]any{sshRulev0, dnsRulev0},
		},
		{
			name: "add one rule",
			args: args{
				oldRules: []any{sshRulev1, dnsRulev1},
				newRules: []any{sshRulev1, httpRulev0, dnsRulev1},
			},
			wantCreate: []map[string]any{httpRulev0},
		},
		{
			name: "add many rules",
			args: args{
				oldRules: []any{sshRulev1},
				newRules: []any{dnsRulev0, sshRulev1, httpRulev0},
			},
			wantCreate: []map[string]any{dnsRulev0, httpRulev0},
		},
		{
			name: "update one rule",
			args: args{
				oldRules: []any{sshRulev1, dnsRulev1},
				newRules: []any{sshRulev2, dnsRulev1},
			},
			wantUpdate: []map[string]any{sshRulev2},
		},
		{
			name: "update many rules",
			args: args{
				oldRules: []any{sshRulev1, dnsRulev1, httpRulev1},
				newRules: []any{sshRulev1, dnsRulev2, httpRulev2},
			},
			wantUpdate: []map[string]any{dnsRulev2, httpRulev2},
		},
		{
			name: "delete one rule",
			args: args{
				oldRules: []any{sshRulev1, dnsRulev1},
				newRules: []any{sshRulev1},
			},
			wantDelete: []map[string]any{dnsRulev1},
		},
		{
			name: "delete many rules",
			args: args{
				oldRules: []any{sshRulev1, dnsRulev1, httpRulev1},
				newRules: []any{dnsRulev1},
			},
			wantDelete: []map[string]any{sshRulev1, httpRulev1},
		},
		{
			name: "create, update, delete",
			args: args{
				oldRules: []any{sshRulev1, dnsRulev1},
				newRules: []any{dnsRulev2, httpRulev0},
			},
			wantCreate: []map[string]any{httpRulev0},
			wantUpdate: []map[string]any{dnsRulev2},
			wantDelete: []map[string]any{sshRulev1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			create, update, del := diffSecurityGroupRules(
				tt.args.oldRules,
				tt.args.newRules,
			)

			assert.ElementsMatch(t, tt.wantCreate, create, "create")
			assert.ElementsMatch(t, tt.wantUpdate, update, "update")
			assert.ElementsMatch(t, tt.wantDelete, del, "delete")
		})
	}
}

func Test_diffSecurityGroupRule(t *testing.T) {
	tests := []struct {
		name    string
		oldRule map[string]any
		newRule map[string]any
		want    bool
	}{
		{
			name:    "both nil",
			oldRule: nil,
			newRule: nil,
			want:    false,
		},
		{
			name:    "old nil",
			oldRule: nil,
			newRule: map[string]any{"notes": "SSH"},
			want:    true,
		},
		{
			name:    "new nil",
			oldRule: map[string]any{"notes": "SSH"},
			newRule: nil,
			want:    true,
		},
		{
			name: "same",
			oldRule: map[string]any{
				"id":       "sgr_8Pwrh8MvC4IeqIUE",
				"protocol": "tcp",
				"ports":    "22,722",
				"targets": schema.NewSet(
					schema.HashString,
					[]any{"all:ipv4", "all:ipv6"},
				),
				"notes": "SSH",
			},
			newRule: map[string]any{
				"id":       "sgr_8Pwrh8MvC4IeqIUE",
				"protocol": "tcp",
				"ports":    "22,722",
				"targets": schema.NewSet(
					schema.HashString,
					[]any{"all:ipv4", "all:ipv6"},
				),
				"notes": "SSH",
			},
			want: false,
		},
		{
			name: "string slice in different order",
			oldRule: map[string]any{
				"id":       "sgr_tNGtTTfpWTfuEfQd",
				"protocol": "tcp",
				"ports":    "22,722",
				"targets": schema.NewSet(
					schema.HashString,
					[]any{"all:ipv4", "all:ipv6"},
				),
				"notes": "SSH",
			},
			newRule: map[string]any{
				"id":       "sgr_tNGtTTfpWTfuEfQd",
				"protocol": "tcp",
				"ports":    "22,722",
				"targets": schema.NewSet(
					schema.HashString,
					[]any{"all:ipv6", "all:ipv4"},
				),
				"notes": "SSH",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := diffSecurityGroupRule(tt.oldRule, tt.newRule)

			assert.Equal(t, tt.want, got)
		})
	}
}
