package v6provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/next/core"
)

func init() { //nolint:gochecknoinits
	resource.AddTestSweepers("katapult_disk", &resource.Sweeper{
		Name: "katapult_disk",
		F:    testSweepDisks,
	})
}

func testSweepDisks(_ string) error {
	m := sweepMeta()
	ctx := context.TODO()

	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		res, err := m.Core.GetOrganizationDisksWithResponse(ctx,
			&core.GetOrganizationDisksParams{
				OrganizationSubDomain: &m.confOrganization,
				Page:                  &pageNum,
			})
		if err != nil {
			return err
		}

		totalPages = res.JSON200.Pagination.TotalPages.MustGet()

		for _, disk := range res.JSON200.Disk {
			if disk.Name == nil {
				continue
			}
			if !strings.HasPrefix(*disk.Name, testAccResourceNamePrefix) {
				continue
			}
			if disk.Id == nil {
				continue
			}

			diskID := *disk.Id
			m.Logger.Info("deleting disk", "id", diskID, "name", *disk.Name)

			delRes, delErr := m.Core.DeleteDiskWithResponse(ctx,
				core.DeleteDiskJSONRequestBody{
					Disk: core.DiskLookup{Id: &diskID},
				})
			if delErr != nil {
				return delErr
			}

			if delRes.JSON200 != nil {
				trashObj := delRes.JSON200.TrashObject
				_, _ = m.Core.DeleteTrashObjectWithResponse(ctx,
					core.DeleteTrashObjectJSONRequestBody{
						TrashObject: core.TrashObjectLookup{
							Id: trashObj.Id,
						},
					})
			}
		}
	}

	return nil
}

func TestAccKatapultDisk_basic(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultDiskDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_disk" "test" {
					  name       = "%s"
					  size_in_gb = 20
					}`, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultDiskExists(
						tt, "katapult_disk.test",
					),
					resource.TestCheckResourceAttr(
						"katapult_disk.test",
						"name", name,
					),
					resource.TestCheckResourceAttr(
						"katapult_disk.test",
						"size_in_gb", "20",
					),
					resource.TestCheckResourceAttrSet(
						"katapult_disk.test", "id",
					),
					resource.TestCheckResourceAttrSet(
						"katapult_disk.test", "wwn",
					),
					resource.TestCheckResourceAttrSet(
						"katapult_disk.test", "state",
					),
					resource.TestCheckResourceAttrSet(
						"katapult_disk.test",
						"storage_speed",
					),
				),
			},
			{
				ResourceName:            "katapult_disk.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"resize_method"},
			},
		},
	})
}

func TestAccKatapultDisk_storage_speed_nvme(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultDiskDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_disk" "test" {
					  name          = "%s"
					  size_in_gb    = 20
					  storage_speed = "nvme"
					}`, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"katapult_disk.test",
						"storage_speed", "nvme",
					),
				),
			},
		},
	})
}

func TestAccKatapultDisk_update(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()
	nameUpdated := name + "-updated"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultDiskDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_disk" "test" {
					  name       = "%s"
					  size_in_gb = 20
					  bus_type   = "virtio"
					}`, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultDiskExists(
						tt, "katapult_disk.test",
					),
					resource.TestCheckResourceAttr(
						"katapult_disk.test", "name", name,
					),
					resource.TestCheckResourceAttr(
						"katapult_disk.test", "bus_type", "virtio",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_disk" "test" {
					  name       = "%s"
					  size_in_gb = 20
					  bus_type   = "scsi"
					}`, nameUpdated,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"katapult_disk.test",
						"name", nameUpdated,
					),
					resource.TestCheckResourceAttr(
						"katapult_disk.test", "bus_type", "scsi",
					),
					// Same resource — ID must not change.
					testAccCheckKatapultDiskExists(
						tt, "katapult_disk.test",
					),
				),
			},
		},
	})
}

func TestAccKatapultDisk_resize(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultDiskDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_disk" "test" {
					  name       = "%s"
					  size_in_gb = 20
					}`, name,
				),
				Check: resource.TestCheckResourceAttr(
					"katapult_disk.test", "size_in_gb", "20",
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_disk" "test" {
					  name          = "%s"
					  size_in_gb    = 30
					  resize_method = "offline"
					}`, name,
				),
				Check: resource.TestCheckResourceAttr(
					"katapult_disk.test", "size_in_gb", "30",
				),
			},
		},
	})
}

//
// Helper check functions
//

func testAccCheckKatapultDiskExists(
	tt *testTools,
	res string,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("resource not found: %s", res)
		}

		diskRes, err := tt.Meta.Core.GetDiskWithResponse(tt.Ctx,
			&core.GetDiskParams{DiskId: &rs.Primary.ID})
		if err != nil {
			return err
		}
		if diskRes.JSON200 == nil {
			return fmt.Errorf(
				"katapult_disk %s not found", rs.Primary.ID,
			)
		}

		return nil
	}
}

func testAccCheckKatapultDiskDestroy(
	tt *testTools,
) resource.TestCheckFunc {
	m := tt.Meta

	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "katapult_disk" {
				continue
			}

			diskRes, err := m.Core.GetDiskWithResponse(tt.Ctx,
				&core.GetDiskParams{DiskId: &rs.Primary.ID})
			if err == nil && diskRes.JSON200 != nil {
				return fmt.Errorf(
					"katapult_disk %s was not destroyed",
					rs.Primary.ID,
				)
			}
			if err != nil && !errors.Is(err, core.ErrNotFound) {
				return err
			}

			trashRes, err := m.Core.GetTrashObjectWithResponse(tt.Ctx,
				&core.GetTrashObjectParams{
					TrashObjectObjectId: &rs.Primary.ID,
				})
			if err == nil && trashRes.JSON200 != nil {
				return fmt.Errorf(
					"katapult_disk %s was deleted "+
						"but not purged from trash",
					rs.Primary.ID,
				)
			}
		}

		return nil
	}
}
