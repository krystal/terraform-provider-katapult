package v6provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/dnaeon/go-vcr/recorder"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/next/core"
)

var securityGroupRuleRepresentationAttributes = []string{
	"inbound_rule", "outbound_rule", "inbound_rules", "outbound_rules",
}

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

	toDelete := []struct{ id, name string }{}
	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		res, err := m.Core.GetOrganizationSecurityGroupsWithResponse(ctx,
			&core.GetOrganizationSecurityGroupsParams{
				OrganizationSubDomain: &m.confOrganization,
				Page:                  &pageNum,
			})
		if err != nil {
			return err
		}
		if res == nil || res.JSON200 == nil {
			return fmt.Errorf("unexpected response listing security groups")
		}

		totalPages, _ = res.JSON200.Pagination.TotalPages.Get()
		for _, sg := range res.JSON200.SecurityGroups {
			if sg.Name != nil && strings.HasPrefix(*sg.Name, testAccResourceNamePrefix) {
				if sg.Id != nil {
					toDelete = append(toDelete, struct{ id, name string }{*sg.Id, *sg.Name})
				}
			}
		}
	}

	for _, sg := range toDelete {
		m.Logger.Info(
			"deleting security group", "id", sg.id, "name", sg.name,
		)
		id := sg.id
		res, err := m.Core.DeleteSecurityGroupWithResponse(ctx,
			core.DeleteSecurityGroupJSONRequestBody{
				SecurityGroup: core.SecurityGroupLookup{Id: &id},
			})
		if err != nil {
			return err
		}
		if res == nil || (res.JSON200 == nil && res.JSON404 == nil && res.StatusCode() != 204) {
			return fmt.Errorf("unexpected response deleting security group %s", sg.id)
		}
	}

	return nil
}

//
// Tests
//

func TestAccKatapultSecurityGroup_example(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultSecurityGroupDestroy(tt),
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
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultSecurityGroupDestroy(tt),
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
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultSecurityGroupDestroy(tt),
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
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultSecurityGroupDestroy(tt),
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
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultSecurityGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_legacy_virtual_machine_group" "web" {
						name = "%s"
					}

					resource "katapult_security_group" "my_sg" {
						name = "%s"
						associations = [katapult_legacy_virtual_machine_group.web.id]
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
					resource "katapult_legacy_virtual_machine_group" "web" {
						name = "%s"
					}

					resource "katapult_legacy_virtual_machine_group" "db" {
						name = "%s-db"
					}

					resource "katapult_security_group" "my_sg" {
						name = "%s"
						associations = [
							katapult_legacy_virtual_machine_group.web.id,
							katapult_legacy_virtual_machine_group.db.id,
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
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultSecurityGroupDestroy(tt),
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
					resource "katapult_legacy_virtual_machine_group" "web" {
						name = "%s"
					}

					resource "katapult_security_group" "my_sg" {
						name = "%s"
						associations = [
							katapult_legacy_virtual_machine_group.web.id,
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
						"outbound_rule.#", "0",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_legacy_virtual_machine_group" "web" {
						name = "%s"
					}

					resource "katapult_legacy_virtual_machine_group" "monitoring" {
						name = "%s"
					}

					resource "katapult_security_group" "my_sg" {
						name = "%s"
						associations = [
							katapult_legacy_virtual_machine_group.web.id,
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
								katapult_legacy_virtual_machine_group.monitoring.id
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
					resource "katapult_legacy_virtual_machine_group" "web" {
						name = "%s"
					}

					resource "katapult_legacy_virtual_machine_group" "db" {
						name = "%s"
					}

					resource "katapult_security_group" "my_sg" {
						name = "%s"
						associations = [
							katapult_legacy_virtual_machine_group.web.id,
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
							targets = [katapult_legacy_virtual_machine_group.db.id]
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
					resource "katapult_legacy_virtual_machine_group" "web" {
						name = "%s"
					}

					resource "katapult_security_group" "my_sg" {
						name = "%s"
						associations = [
							katapult_legacy_virtual_machine_group.web.id,
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
					resource "katapult_legacy_virtual_machine_group" "web" {
						name = "%s"
					}

					resource "katapult_security_group" "my_sg" {
						name = "%s"
						associations = [
							katapult_legacy_virtual_machine_group.web.id,
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
				ResourceName:            "katapult_security_group.my_sg",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: securityGroupRuleRepresentationAttributes,
			},
		},
	})
}

func TestAccKatapultSecurityGroup_dynamic_rules(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultSecurityGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
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
				ResourceName:            "katapult_security_group.my_sg",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: securityGroupRuleRepresentationAttributes,
			},
		},
	})
}

func TestAccKatapultSecurityGroup_invalid_rules(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultSecurityGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name         = "%s"
						associations = [""]
					}`,
					name,
				),
				ExpectError: regexp.MustCompile("Invalid Attribute Value Length"),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"

						inbound_rule {
							protocol = "tcp"
							targets  = [""]
						}
					}`,
					name,
				),
				ExpectError: regexp.MustCompile("Invalid Attribute Value Length"),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "my_sg" {
						name = "%s"

						outbound_rules = [{
							protocol = "udp"
							targets  = [null]
						}]
					}`,
					name,
				),
				ExpectError: regexp.MustCompile("Null Set Value"),
			},
			{
				Config: undent.Stringf(`
					resource "terraform_data" "external_rules" {
						input = true
					}

					resource "katapult_security_group" "my_sg" {
						name           = "%s"
						external_rules = terraform_data.external_rules.output

						inbound_rules = [{
							protocol = "tcp"
							targets  = ["all:ipv4"]
						}]
					}`,
					name,
				),
				ExpectError: regexp.MustCompile("Conflicting Security Group Rules"),
			},
			{Config: "terraform {}"},
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
					"Conflicting Security Group Rules",
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
					"Conflicting Security Group Rules",
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
					"Conflicting Security Group Rules",
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
					"Conflicting Security Group Rules",
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
					"Invalid Attribute Value Match",
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
					"Invalid Attribute Value Match",
				),
			},
		},
	})
}

func TestAccKatapultSecurityGroup_multiple(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultSecurityGroupDestroy(tt),
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
				ResourceName:            "katapult_security_group.my_sg_foo",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: securityGroupRuleRepresentationAttributes,
			},
			{
				ResourceName:            "katapult_security_group.my_sg_bar",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: securityGroupRuleRepresentationAttributes,
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

			attempts := 1
			if tt.Recorder != nil && tt.Recorder.Mode() == recorder.ModeReplaying {
				// Framework refreshes can use fewer repeated legacy GETs than the
				// moved cassette. Consume those successful observations until the
				// recorded not-found response proves deletion.
				attempts = 100
			}
			destroyed := false
			for range attempts {
				id := rs.Primary.ID
				res, err := m.Core.GetSecurityGroupWithResponse(
					tt.Ctx, &core.GetSecurityGroupParams{SecurityGroupId: &id},
				)
				if errors.Is(err, core.ErrNotFound) || res != nil && res.JSON404 != nil {
					destroyed = true
					break
				}
				if err != nil {
					return err
				}
				if attempts == 1 {
					return fmt.Errorf("katapult_security_group %s was not destroyed", id)
				}
			}
			if !destroyed {
				return fmt.Errorf("katapult_security_group %s was not destroyed", rs.Primary.ID)
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

		id := rs.Primary.ID
		response, err := m.Core.GetSecurityGroupWithResponse(
			tt.Ctx, &core.GetSecurityGroupParams{SecurityGroupId: &id},
		)
		if err != nil {
			return err
		}
		if response == nil || response.JSON200 == nil || response.JSON200.SecurityGroup.Id == nil {
			return fmt.Errorf("security group %s not found", id)
		}

		return resource.TestCheckResourceAttr(res, "id", *response.JSON200.SecurityGroup.Id)(s)
	}
}
