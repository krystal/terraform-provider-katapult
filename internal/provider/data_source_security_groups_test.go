package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceSecurityGroups_default(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		CheckDestroy:      testAccCheckKatapultSecurityGroupDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "first" {
						name = "%s"
						allow_all_inbound = true
						allow_all_outbound = false
					}

					resource "katapult_security_group" "second" {
						name = "%s"
						allow_all_inbound = false
						allow_all_outbound = true

						# Ensure consistent ordering for testing purposes.
						depends_on = [katapult_security_group.first]
					}`,
					name+"-1", name+"-2",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultSecurityGroupExists(
						tt, "katapult_security_group.first",
					),
					testAccCheckKatapultSecurityGroupExists(
						tt, "katapult_security_group.second",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_security_group" "first" {
						name = "%s"
						allow_all_inbound = true
						allow_all_outbound = false
					}

					resource "katapult_security_group" "second" {
						name = "%s"
						allow_all_inbound = false
						allow_all_outbound = true

						# Ensure consistent ordering for testing purposes.
						depends_on = [katapult_security_group.first]
					}

					data "katapult_security_groups" "all" {}`,
					name+"-1", name+"-2",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.katapult_security_groups.all",
						"security_groups.0.name", name+"-1",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_security_groups.all",
						"security_groups.0.allow_all_inbound", "true",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_security_groups.all",
						"security_groups.0.allow_all_outbound", "false",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_security_groups.all",
						"security_groups.1.name", name+"-2",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_security_groups.all",
						"security_groups.1.allow_all_inbound", "false",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_security_groups.all",
						"security_groups.1.allow_all_outbound", "true",
					),
				),
			},
		},
	})
}
