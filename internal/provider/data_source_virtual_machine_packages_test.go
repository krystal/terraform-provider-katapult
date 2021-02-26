package provider

import (
	"errors"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/krystal/go-katapult/pkg/katapult"
	"github.com/stretchr/testify/require"
)

func TestAccKatapultDataSourceVirtualMachinePackages_all(t *testing.T) {
	tt := newTestTools(t)

	pkgs, err := testHelperFetchAllVirtualMachinePackages(tt)
	require.NoError(t, err)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "katapult_virtual_machine_packages" "all" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVirtualMachinePackages(
						tt, "data.katapult_virtual_machine_packages.all", pkgs,
					),
				),
			},
		},
	})
}

//
// Helpers
//

func testHelperFetchAllVirtualMachinePackages(
	tt *testTools,
) ([]*katapult.VirtualMachinePackage, error) {
	var pkgs []*katapult.VirtualMachinePackage
	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		pageResult, resp, err := tt.Meta.Client.VirtualMachinePackages.List(
			tt.Ctx, &katapult.ListOptions{Page: pageNum},
		)
		if err != nil {
			return nil, err
		}

		totalPages = resp.Pagination.TotalPages
		pkgs = append(pkgs, pageResult...)
	}

	if len(pkgs) == 0 {
		return nil, errors.New("no virtual machine packages found")
	}

	return pkgs, nil
}

func testAccCheckKatapultVirtualMachinePackages(
	tt *testTools,
	res string,
	pkgs []*katapult.VirtualMachinePackage,
) resource.TestCheckFunc {
	tfs := []resource.TestCheckFunc{}

	for i, pkg := range pkgs {
		prefix := fmt.Sprintf("packages.%d.", i)
		tfs = append(tfs, testAccCheckKatapultVirtualMachinePackageAttrs(
			tt, res, pkg, prefix,
		))
	}

	return resource.ComposeAggregateTestCheckFunc(tfs...)
}