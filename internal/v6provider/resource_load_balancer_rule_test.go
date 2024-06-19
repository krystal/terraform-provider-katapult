package v6provider

import (
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
)

// Test with minimal required configuration.
func TestAccKatapultLoadBalancerRule_minimal(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultLoadBalancerDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "my_lb" {
					  name = "%s"
					}

					resource "katapult_load_balancer_rule" "my_rule" {
						load_balancer_id = katapult_load_balancer.my_lb.id
						destination_port = 8080
						listen_port = 80
						protocol = "HTTP"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerRuleAttrs(
						tt, "katapult_load_balancer_rule.my_rule",
					),
					resource.TestCheckResourceAttrPair(
						"katapult_load_balancer_rule.my_rule",
						"load_balancer_id",
						"katapult_load_balancer.my_lb",
						"id",
					),
					// Verify default values for non-required attributes.
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.my_rule",
						"backend_ssl",
						"false",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.my_rule",
						"passthrough_ssl",
						"false",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.my_rule",
						"proxy_protocol",
						"false",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.my_rule",
						"check_enabled",
						"false",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.my_rule",
						"check_fall",
						"2",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.my_rule",
						"check_interval",
						"20",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.my_rule",
						"check_http_statuses",
						"2",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.my_rule",
						"check_path",
						"/",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.my_rule",
						"check_protocol",
						"HTTP",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.my_rule",
						"check_rise",
						"2",
					),
					resource.TestCheckResourceAttr(
						"katapult_load_balancer_rule.my_rule",
						"check_timeout",
						"5",
					),
				),
			},
			{
				ResourceName:      "katapult_load_balancer_rule.my_rule",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultLoadBalancerRule_multi(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultLoadBalancerDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "my_lb" {
					  name = "%s"
					}

					resource "katapult_load_balancer_rule" "http" {
						load_balancer_id = katapult_load_balancer.my_lb.id
						destination_port = 8080
						listen_port = 80
						protocol = "HTTP"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerRuleAttrs(
						tt, "katapult_load_balancer_rule.http",
					),
					resource.TestCheckResourceAttrPair(
						"katapult_load_balancer_rule.http",
						"load_balancer_id",
						"katapult_load_balancer.my_lb",
						"id",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "my_lb" {
					  name = "%s"
					}

					resource "katapult_load_balancer_rule" "http" {
						load_balancer_id = katapult_load_balancer.my_lb.id
						destination_port = 8080
						listen_port = 80
						protocol = "HTTP"
					}

					resource "katapult_load_balancer_rule" "https" {
						# Ensure HTTP rule is created first
						depends_on = [katapult_load_balancer_rule.http]
						load_balancer_id = katapult_load_balancer.my_lb.id
						destination_port = 8443
						listen_port = 443
						protocol = "HTTPS"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerRuleAttrs(
						tt, "katapult_load_balancer_rule.http",
					),
					resource.TestCheckResourceAttrPair(
						"katapult_load_balancer_rule.http",
						"load_balancer_id",
						"katapult_load_balancer.my_lb",
						"id",
					),
					testAccCheckKatapultLoadBalancerRuleAttrs(
						tt, "katapult_load_balancer_rule.https",
					),
					resource.TestCheckResourceAttrPair(
						"katapult_load_balancer_rule.https",
						"load_balancer_id",
						"katapult_load_balancer.my_lb",
						"id",
					),
				),
			},
			{
				ResourceName:      "katapult_load_balancer_rule.http",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// Explicitly test all attributes with non-default values, and verify they can
// be modified.
func TestAccKatapultLoadBalancerRule_full(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultLoadBalancerDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "my_lb" {
					  name = "%s"
					}

					resource "katapult_load_balancer_rule" "my_rule" {
						load_balancer_id = katapult_load_balancer.my_lb.id
						destination_port = 8080
						listen_port = 80
						protocol = "HTTP"
						backend_ssl = true
						passthrough_ssl = false
						proxy_protocol = true
						check_enabled = false
						check_fall = 3
						check_interval = 30
						check_http_statuses = "23"
						check_path = "/healthz"
						check_protocol = "HTTP"
						check_rise = 3
						check_timeout = 15
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerRuleAttrs(
						tt, "katapult_load_balancer_rule.my_rule",
					),
					resource.TestCheckResourceAttrPair(
						"katapult_load_balancer_rule.my_rule",
						"load_balancer_id",
						"katapult_load_balancer.my_lb",
						"id",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "my_lb" {
					  name = "%s"
					}

					resource "katapult_load_balancer_rule" "my_rule" {
						load_balancer_id = katapult_load_balancer.my_lb.id
						destination_port = 8443
						listen_port = 443
						protocol = "HTTPS"
						backend_ssl = false
						passthrough_ssl = true
						proxy_protocol = false
						check_enabled = true
						check_fall = 4
						check_interval = 60
						check_http_statuses = "234"
						check_path = "/health"
						check_protocol = "HTTP"
						check_rise = 2
						check_timeout = 20
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerRuleAttrs(
						tt, "katapult_load_balancer_rule.my_rule",
					),
					resource.TestCheckResourceAttrPair(
						"katapult_load_balancer_rule.my_rule",
						"load_balancer_id",
						"katapult_load_balancer.my_lb",
						"id",
					),
				),
			},
			{
				ResourceName:      "katapult_load_balancer_rule.my_rule",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultLoadBalancerRule_move(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	var oldID, currentID string

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultLoadBalancerDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "my_lb1" {
					  name = "%s"
					}

					resource "katapult_load_balancer_rule" "my_rule" {
						load_balancer_id = katapult_load_balancer.my_lb1.id
						destination_port = 8080
						listen_port = 80
						protocol = "HTTP"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerRuleExists(
						tt, "katapult_load_balancer_rule.my_rule", &oldID,
					),
					resource.TestCheckResourceAttrPair(
						"katapult_load_balancer_rule.my_rule",
						"load_balancer_id",
						"katapult_load_balancer.my_lb1",
						"id",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "my_lb1" {
					  name = "%s"
					}

					resource "katapult_load_balancer" "my_lb2" {
					  name = "%s"
					}

					resource "katapult_load_balancer_rule" "my_rule" {
						load_balancer_id = katapult_load_balancer.my_lb2.id
						destination_port = 8080
						listen_port = 80
						protocol = "HTTP"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultLoadBalancerRuleExists(
						tt, "katapult_load_balancer_rule.my_rule", &currentID,
					),
					resource.TestCheckResourceAttrPair(
						"katapult_load_balancer_rule.my_rule",
						"load_balancer_id",
						"katapult_load_balancer.my_lb2",
						"id",
					),
					testAccCheckResourceAttrChanged(
						"katapult_load_balancer_rule.my_rule", "id",
						&oldID, &currentID,
					),
				),
			},
			{
				ResourceName:      "katapult_load_balancer_rule.my_rule",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultLoadBalancerRule_invalid(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultLoadBalancerDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "my_lb" {
					  name = "%s"
					}

					resource "katapult_load_balancer_rule" "my_rule" {
						load_balancer_id = katapult_load_balancer.my_lb.id
						destination_port = 8080
						listen_port = 443
						protocol = "HTTPS"
						passthrough_ssl = false
						check_enabled = true
						check_protocol = "HTTPS"
						check_path = "/"
					}`,
					name,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						"check_path cannot be set " +
							"if check_protocol is not HTTP",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "my_lb" {
					  name = "%s"
					}

					resource "katapult_load_balancer_rule" "my_rule" {
						load_balancer_id = katapult_load_balancer.my_lb.id
						destination_port = 8080
						listen_port = 80
						protocol = "HTTP"
						passthrough_ssl = false
						check_enabled = true
						check_protocol = "HTTPS"
						check_http_statuses = "2"
					}`,
					name,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						"check_http_statuses cannot be set " +
							"if check_protocol is not HTTP",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "my_lb" {
					  name = "%s"
					}

					resource "katapult_load_balancer_rule" "my_rule" {
						load_balancer_id = katapult_load_balancer.my_lb.id
						destination_port = 8080
						listen_port = 80
						protocol = "HTTP"
						passthrough_ssl = false
						certificate_ids = ["cert_123"]
					}`,
					name,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						"certificate_ids cannot be set " +
							"if protocol is not HTTPS",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "my_lb" {
					  name = "%s"
					}

					resource "katapult_load_balancer_rule" "my_rule" {
						load_balancer_id = katapult_load_balancer.my_lb.id
						destination_port = 8080
						listen_port = 80
						protocol = "HTTP"
						passthrough_ssl = true
					}`,
					name,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						"passthrough_ssl cannot be set " +
							"if protocol is not HTTPS",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "my_lb" {
					  name = "%s"
					}

					resource "katapult_load_balancer_rule" "my_rule" {
						load_balancer_id = katapult_load_balancer.my_lb.id
						destination_port = 8080
						listen_port = 80
						protocol = "ICMP"
						passthrough_ssl = false
					}`,
					name,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						"protocol value must be one of: " +
							`["HTTP" "HTTPS" "TCP"], ` +
							`got: "ICMP"`,
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_load_balancer" "my_lb" {
					  name = "%s"
					}

					resource "katapult_load_balancer_rule" "my_rule" {
						load_balancer_id = katapult_load_balancer.my_lb.id
						destination_port = 8080
						listen_port = 80
						protocol = "HTTP"
						passthrough_ssl = false
						check_enabled = true
						check_protocol = "ICMP"
					}`,
					name,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						"check_protocol value must be one of: " +
							`["HTTP" "HTTPS" "TCP"]`,
					),
				),
			},
		},
	})
}

//
// Helpers
//

func testAccCheckKatapultLoadBalancerRuleExists(
	tt *testTools,
	res string,
	id *string,
) resource.TestCheckFunc {
	m := tt.Meta

	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("resource not found: %s", res)
		}

		lbr, _, err := m.Core.LoadBalancerRules.GetByID(
			tt.Ctx, rs.Primary.ID,
		)
		if err != nil {
			return err
		}

		// Expose ID if provided by caller.
		if id != nil {
			*id = lbr.ID
		}

		return resource.TestCheckResourceAttr(res, "id", lbr.ID)(s)
	}
}

func testAccCheckKatapultLoadBalancerRuleAttrs(
	tt *testTools,
	res string,
) resource.TestCheckFunc {
	m := tt.Meta

	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("resource not found: %s", res)
		}

		lbr, _, err := m.Core.LoadBalancerRules.GetByID(
			tt.Ctx, rs.Primary.ID,
		)
		if err != nil {
			return err
		}

		tfs := []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(
				res, "id", lbr.ID,
			),
			resource.TestCheckResourceAttr(
				res, "load_balancer_id", lbr.LoadBalancer.ID,
			),
			resource.TestCheckResourceAttr(
				res, "destination_port", strconv.Itoa(lbr.DestinationPort),
			),
			resource.TestCheckResourceAttr(
				res, "listen_port", strconv.Itoa(lbr.ListenPort),
			),
			resource.TestCheckResourceAttr(
				res, "protocol", string(lbr.Protocol),
			),
			resource.TestCheckResourceAttr(
				res, "certificate_ids.#", strconv.Itoa(len(lbr.Certificates)),
			),
			resource.TestCheckResourceAttr(
				res, "proxy_protocol", strconv.FormatBool(lbr.ProxyProtocol),
			),
			resource.TestCheckResourceAttr(
				res, "backend_ssl", strconv.FormatBool(lbr.BackendSSL),
			),
			resource.TestCheckResourceAttr(
				res, "passthrough_ssl", strconv.FormatBool(lbr.PassthroughSSL),
			),
			resource.TestCheckResourceAttr(
				res, "check_enabled", strconv.FormatBool(lbr.CheckEnabled),
			),
			resource.TestCheckResourceAttr(
				res, "check_fall", strconv.Itoa(lbr.CheckFall),
			),
			resource.TestCheckResourceAttr(
				res, "check_interval", strconv.Itoa(lbr.CheckInterval),
			),
			resource.TestCheckResourceAttr(
				res, "check_http_statuses", string(lbr.CheckHTTPStatuses),
			),
			resource.TestCheckResourceAttr(
				res, "check_path", lbr.CheckPath,
			),
			resource.TestCheckResourceAttr(
				res, "check_protocol", string(lbr.CheckProtocol),
			),
			resource.TestCheckResourceAttr(
				res, "check_rise", strconv.Itoa(lbr.CheckRise),
			),
			resource.TestCheckResourceAttr(
				res, "check_timeout", strconv.Itoa(lbr.CheckTimeout),
			),
		}

		for _, cert := range lbr.Certificates {
			tfs = append(tfs,
				resource.TestCheckTypeSetElemAttr(
					res, "certificate_ids.*", cert.ID,
				),
			)
		}

		return resource.ComposeAggregateTestCheckFunc(tfs...)(s)
	}
}

func testAccCheckResourceAttrChanged(
	res, attr string,
	oldValue, currentValue *string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		if *oldValue == *currentValue {
			return fmt.Errorf(
				"Expected resource %q attribute %q to change, but it did NOT",
				res, attr,
			)
		}

		return nil
	}
}

func testAccCheckResourceAttrNotChanged(
	res, attr string,
	oldValue, currentValue *string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		if *oldValue != *currentValue {
			return fmt.Errorf(
				"Expected resource %q attribute %q to NOT change, but it did",
				res, attr,
			)
		}

		return nil
	}
}
