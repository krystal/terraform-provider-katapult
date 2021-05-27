package provider

import (
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/core"
	"github.com/stretchr/testify/require"
)

func TestAccKatapultDataSourceVirtualMachinePackage_by_id(t *testing.T) {
	tt := newTestTools(t)

	pkg, _, err := tt.Meta.Core.VirtualMachinePackages.GetByPermalink(
		tt.Ctx, "rock-6",
	)
	require.NoError(t, err)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					data "katapult_virtual_machine_package" "main" {
					  id = "%s"
					}`,
					pkg.ID,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVirtualMachinePackageExists(
						tt, "data.katapult_virtual_machine_package.main",
					),
					testAccCheckKatapultVirtualMachinePackageAttrs(
						tt, "data.katapult_virtual_machine_package.main",
						pkg, "",
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceVirtualMachinePackage_by_permalink(t *testing.T) {
	tt := newTestTools(t)

	pkg, _, err := tt.Meta.Core.VirtualMachinePackages.GetByPermalink(
		tt.Ctx, "rock-3",
	)
	require.NoError(t, err)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					data "katapult_virtual_machine_package" "main" {
					  permalink = "%s"
					}`,
					pkg.Permalink,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultVirtualMachinePackageExists(
						tt, "data.katapult_virtual_machine_package.main",
					),
					testAccCheckKatapultVirtualMachinePackageAttrs(
						tt, "data.katapult_virtual_machine_package.main",
						pkg, "",
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceVirtualMachinePackage_blank(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: tt.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "katapult_virtual_machine_package" "main" {}`,
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta("one of `id,permalink` must be specified"),
				),
			},
		},
	})
}

//
// Helpers
//

func testAccCheckKatapultVirtualMachinePackageExists(
	tt *testTools,
	res string,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := tt.Meta.Core

		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("resource not found: %s", res)
		}

		_, _, err := c.VirtualMachinePackages.GetByID(
			tt.Ctx, rs.Primary.ID,
		)
		if err != nil {
			return err
		}

		return nil
	}
}

func testAccCheckKatapultVirtualMachinePackageAttrs(
	tt *testTools,
	res string,
	pkg *core.VirtualMachinePackage,
	prefix string,
) resource.TestCheckFunc {
	return resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckResourceAttr(res, prefix+"id", pkg.ID),
		resource.TestCheckResourceAttr(res, prefix+"name", pkg.Name),
		resource.TestCheckResourceAttr(res, prefix+"permalink", pkg.Permalink),
		resource.TestCheckResourceAttr(
			res, prefix+"cpu_cores", strconv.Itoa(pkg.CPUCores),
		),
		resource.TestCheckResourceAttr(
			res, prefix+"ipv4_addresses", strconv.Itoa(pkg.IPv4Addresses),
		),
		resource.TestCheckResourceAttr(
			res, prefix+"memory_in_gb", strconv.Itoa(pkg.MemoryInGB),
		),
		resource.TestCheckResourceAttr(
			res, prefix+"storage_in_gb", strconv.Itoa(pkg.StorageInGB),
		),
		resource.TestCheckResourceAttr(res, prefix+"privacy", pkg.Privacy),
	)
}
