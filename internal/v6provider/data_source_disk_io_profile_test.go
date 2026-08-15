package v6provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jimeh/undent"
)

func TestAccKatapultDataSourceDiskIOProfile_selectors(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      `data "katapult_disk_io_profile" "selected" {}`,
				ExpectError: regexp.MustCompile(`(?i)exactly one.*id,permalink`),
			},
			{
				Config: undent.String(`
					data "katapult_disk_io_profile" "selected" {
					  id = "iop_missing"
					}`),
				ExpectError: regexp.MustCompile(
					`No disk I/O profile with id "iop_missing" exists`,
				),
			},
			{
				Config: undent.String(`
					data "katapult_disk_io_profile" "selected" {
					  id        = "iop_invalid"
					  permalink = "invalid"
					}`),
				ExpectError: regexp.MustCompile(`(?i)exactly one.*id,permalink`),
			},
			{
				Config: undent.String(`
					data "katapult_disk_io_profiles" "all" {}

					data "katapult_disk_io_profile" "by_id" {
					  id = data.katapult_disk_io_profiles.all.profiles[0].id
					}

					data "katapult_disk_io_profile" "by_permalink" {
					  permalink = data.katapult_disk_io_profiles.all.profiles[0].permalink
					}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						"data.katapult_disk_io_profile.by_id", "id",
						"data.katapult_disk_io_profiles.all", "profiles.0.id",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_disk_io_profile.by_id", "permalink",
						"data.katapult_disk_io_profiles.all", "profiles.0.permalink",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_disk_io_profile.by_permalink", "id",
						"data.katapult_disk_io_profiles.all", "profiles.0.id",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_disk_io_profile.by_permalink", "name",
						"data.katapult_disk_io_profile.by_id", "name",
					),
				),
			},
		},
	})
}
