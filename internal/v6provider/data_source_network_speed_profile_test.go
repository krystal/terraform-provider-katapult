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
					}

					data "katapult_network_speed_profile" "empty_id" {
					  id        = ""
					  permalink = data.katapult_network_speed_profiles.all.profiles[0].permalink
					}

					data "katapult_network_speed_profile" "empty_permalink" {
					  id        = data.katapult_network_speed_profiles.all.profiles[0].id
					  permalink = ""
					}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.katapult_network_speed_profiles.all",
						"id", tt.Meta.confOrganization,
					),
					resource.TestCheckResourceAttrSet(
						"data.katapult_network_speed_profiles.all", "profiles.0.id",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_network_speed_profiles.all",
						"profiles.1.download_speed", "0",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_network_speed_profiles.all",
						"profiles.3.upload_speed", "0",
					),
					resource.TestCheckResourceAttr(
						"data.katapult_network_speed_profiles.all",
						"profiles.3.download_speed", "0",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_network_speed_profile.by_id", "id",
						"data.katapult_network_speed_profiles.all", "profiles.0.id",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_network_speed_profile.by_id", "name",
						"data.katapult_network_speed_profiles.all", "profiles.0.name",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_network_speed_profile.by_id", "permalink",
						"data.katapult_network_speed_profiles.all", "profiles.0.permalink",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_network_speed_profile.by_id", "upload_speed",
						"data.katapult_network_speed_profiles.all", "profiles.0.upload_speed",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_network_speed_profile.by_id", "download_speed",
						"data.katapult_network_speed_profiles.all", "profiles.0.download_speed",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_network_speed_profile.by_permalink", "id",
						"data.katapult_network_speed_profiles.all", "profiles.0.id",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_network_speed_profile.both", "id",
						"data.katapult_network_speed_profiles.all", "profiles.0.id",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_network_speed_profile.empty_id", "id",
						"data.katapult_network_speed_profiles.all", "profiles.0.id",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_network_speed_profile.empty_permalink", "id",
						"data.katapult_network_speed_profiles.all", "profiles.0.id",
					),
				),
			},
		},
	})
}
