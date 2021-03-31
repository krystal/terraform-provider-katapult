package provider

import (
	"regexp"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/jimeh/undent"
	"github.com/stretchr/testify/require"
)

func TestAccKatapultDataSourceNetworkSpeedProfile_by_id(t *testing.T) {
	tt := newTestTools(t)

	profiles, err := testHelperFetchAllNetworkSpeedProfiles(tt)
	require.NoError(t, err)
	require.Greater(t, len(profiles), 0)

	profile := profiles[0]

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					data "katapult_network_speed_profile" "main" {
					  id = "%s"
					}`,
					profile.ID,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.katapult_network_speed_profile.main",
						"id", profile.ID,
					),
					resource.TestCheckResourceAttr(
						"data.katapult_network_speed_profile.main",
						"name", profile.Name,
					),
					resource.TestCheckResourceAttr(
						"data.katapult_network_speed_profile.main",
						"permalink", profile.Permalink,
					),
					resource.TestCheckResourceAttr(
						"data.katapult_network_speed_profile.main",
						"upload_speed", strconv.Itoa(profile.UploadSpeedInMbit),
					),
					resource.TestCheckResourceAttr(
						"data.katapult_network_speed_profile.main",
						"download_speed",
						strconv.Itoa(profile.DownloadSpeedInMbit),
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceNetworkSpeedProfile_by_permalink(t *testing.T) {
	tt := newTestTools(t)

	profiles, err := testHelperFetchAllNetworkSpeedProfiles(tt)
	require.NoError(t, err)
	require.Greater(t, len(profiles), 0)

	profile := profiles[0]

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					data "katapult_network_speed_profile" "main" {
					  permalink = "%s"
					}`,
					profile.Permalink,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.katapult_network_speed_profile.main",
						"id", profile.ID,
					),
					resource.TestCheckResourceAttr(
						"data.katapult_network_speed_profile.main",
						"name", profile.Name,
					),
					resource.TestCheckResourceAttr(
						"data.katapult_network_speed_profile.main",
						"permalink", profile.Permalink,
					),
					resource.TestCheckResourceAttr(
						"data.katapult_network_speed_profile.main",
						"upload_speed", strconv.Itoa(profile.UploadSpeedInMbit),
					),
					resource.TestCheckResourceAttr(
						"data.katapult_network_speed_profile.main",
						"download_speed",
						strconv.Itoa(profile.DownloadSpeedInMbit),
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceNetworkSpeedProfile_blank(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "katapult_network_speed_profile" "main" {}`,
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta("one of `id,permalink` must be specified"),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceNetworkSpeedProfile_invalid(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					data "katapult_network_speed_profile" "main" {
					  name = "Ubuntu 20.04"
					}`,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta("one of `id,permalink` must be specified"),
				),
			},
		},
	})
}
