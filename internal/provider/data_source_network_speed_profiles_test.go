package provider

import (
	"errors"
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/krystal/go-katapult/core"
	"github.com/stretchr/testify/require"
)

func TestAccKatapultDataSourceNetworkSpeedProfiles_all(t *testing.T) {
	tt := newTestTools(t)

	profiles, err := testHelperFetchAllNetworkSpeedProfiles(tt)
	require.NoError(t, err)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "katapult_network_speed_profiles" "main" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultNetworkSpeedProfiles(
						"data.katapult_network_speed_profiles.main",
						profiles,
					),
				),
			},
		},
	})
}

//
// Helpers
//

func testHelperFetchAllNetworkSpeedProfiles(
	tt *testTools,
) ([]*core.NetworkSpeedProfile, error) {
	var profiles []*core.NetworkSpeedProfile
	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		pageResult, resp, err := tt.Meta.Core.NetworkSpeedProfiles.List(
			tt.Ctx, tt.Meta.OrganizationRef,
			&core.ListOptions{Page: pageNum},
		)
		if err != nil {
			return nil, err
		}

		totalPages = resp.Pagination.TotalPages
		profiles = append(profiles, pageResult...)
	}

	if len(profiles) == 0 {
		return nil, errors.New("no network speed profiles found")
	}

	return profiles, nil
}

func testAccCheckKatapultNetworkSpeedProfiles(
	res string,
	profiles []*core.NetworkSpeedProfile,
) resource.TestCheckFunc {
	tfs := []resource.TestCheckFunc{}

	for i, profile := range profiles {
		prefix := fmt.Sprintf("profiles.%d.", i)
		tfs = append(tfs,
			resource.TestCheckResourceAttr(
				res, prefix+"id", profile.ID,
			),
			resource.TestCheckResourceAttr(
				res, prefix+"name", profile.Name,
			),
			resource.TestCheckResourceAttr(
				res, prefix+"permalink", profile.Permalink,
			),
			resource.TestCheckResourceAttr(
				res, prefix+"upload_speed",
				strconv.Itoa(profile.UploadSpeedInMbit),
			),
			resource.TestCheckResourceAttr(
				res, prefix+"download_speed",
				strconv.Itoa(profile.DownloadSpeedInMbit),
			),
		)
	}

	return resource.ComposeAggregateTestCheckFunc(tfs...)
}
