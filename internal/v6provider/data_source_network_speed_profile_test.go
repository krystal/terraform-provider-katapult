package v6provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceNetworkSpeedProfile_selectors(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      `data "katapult_network_speed_profile" "selected" {}`,
				ExpectError: regexp.MustCompile(`(?i)at least one.*id,permalink`),
			},
			{
				Config: undent.String(`
					data "katapult_network_speed_profiles" "all" {}

					data "katapult_network_speed_profile" "by_id" {
					  id = data.katapult_network_speed_profiles.all.profiles[0].id
					}

					data "katapult_network_speed_profile" "by_permalink" {
					  permalink = data.katapult_network_speed_profiles.all.profiles[0].permalink
					}

					data "katapult_network_speed_profile" "both" {
					  id        = data.katapult_network_speed_profiles.all.profiles[0].id
					  permalink = "ignored"
					}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.katapult_network_speed_profiles.all",
						"id", tt.Meta.confOrganization,
					),
					resource.TestCheckResourceAttrSet(
						"data.katapult_network_speed_profiles.all", "profiles.0.id",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_network_speed_profile.by_id", "id",
						"data.katapult_network_speed_profiles.all", "profiles.0.id",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_network_speed_profile.by_permalink", "id",
						"data.katapult_network_speed_profiles.all", "profiles.0.id",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_network_speed_profile.both", "id",
						"data.katapult_network_speed_profiles.all", "profiles.0.id",
					),
				),
			},
		},
	})
}
