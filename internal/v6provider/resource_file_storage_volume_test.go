package v6provider

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult"
	"github.com/krystal/go-katapult/core"
)

func init() { //nolint:gochecknoinits
	resource.AddTestSweepers("katapult_file_storage_volume", &resource.Sweeper{
		Name: "katapult_file_storage_volume",
		F:    testSweepFileStorageVolumes,
	})
}

func testSweepFileStorageVolumes(_ string) error {
	m := sweepMeta()
	ctx := context.Background()

	toDelete := []*core.FileStorageVolume{}
	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		pageResults, resp, err := m.Core.FileStorageVolumes.List(
			ctx, m.OrganizationRef, &core.ListOptions{Page: pageNum},
		)
		if err != nil {
			return err
		}

		totalPages = resp.Pagination.TotalPages
		for _, fsv := range pageResults {
			if strings.HasPrefix(fsv.Name, testAccResourceNamePrefix) {
				toDelete = append(toDelete, fsv)
			}
		}
	}

	for _, fsv := range toDelete {
		m.Logger.Info("deleting file storage volume",
			"id", fsv.ID, "name", fsv.Name,
		)
		_, trash, _, err := m.Core.FileStorageVolumes.Delete(ctx, fsv.Ref())
		if err != nil {
			return err
		}

		m.Logger.Info("purging file storage volume",
			"id", fsv.ID, "name", fsv.Name,
		)

		trashRef := trash.Ref()
		_, _, err = m.Core.TrashObjects.Purge(ctx, trashRef)
		if err != nil {
			return err
		}

		waiter := &resource.StateChangeConf{
			Pending: []string{"exists"},
			Target:  []string{"not_found"},
			Refresh: func() (interface{}, string, error) {
				_, _, e := m.Core.TrashObjects.Get(ctx, trashRef)
				if e != nil && errors.Is(e, katapult.ErrNotFound) {
					return 1, "not_found", nil
				}

				return nil, "exists", nil
			},
			Timeout:                   5 * time.Minute,
			Delay:                     2 * time.Second,
			MinTimeout:                5 * time.Second,
			ContinuousTargetOccurence: 1,
		}

		_, err = waiter.WaitForStateContext(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}

//
// Tests
//

func TestAccKatapultFileStorageVolume_minimal(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultFSVDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_file_storage_volume" "my_fsv" {
						name = "%s"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultFileStorageVolumeAttrs(
						tt, "katapult_file_storage_volume.my_fsv",
					),
					resource.TestCheckResourceAttr(
						"katapult_file_storage_volume.my_fsv",
						"associations.#", "0",
					),
				),
			},
			{
				ResourceName:      "katapult_file_storage_volume.my_fsv",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccKatapultFileStorageVolume_update_name(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultFSVDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_file_storage_volume" "my_fsv" {
						name = "%s"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultFileStorageVolumeAttrs(
						tt, "katapult_file_storage_volume.my_fsv",
					),
					resource.TestCheckResourceAttr(
						"katapult_file_storage_volume.my_fsv",
						"associations.#", "0",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_file_storage_volume" "my_fsv" {
						name = "%s-foobar"
					}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultFileStorageVolumeAttrs(
						tt, "katapult_file_storage_volume.my_fsv",
					),
					resource.TestCheckResourceAttr(
						"katapult_file_storage_volume.my_fsv",
						"name", name+"-foobar",
					),
				),
			},
			{
				ResourceName:      "katapult_file_storage_volume.my_fsv",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TODO: RE-ADD THIS TEST WHEN VMS ARE MIGRATED TO V6
// func TestAccKatapultFileStorageVolume_associations(t *testing.T) {
// 	tt := newTestTools(t)

// 	name := tt.ResourceName()

// 	resource.ParallelTest(t, resource.TestCase{
// 		PreCheck:                 func() { testAccPreCheck(t) },
// 		ProtoV6ProviderFactories: tt.ProviderFactories,
// 		CheckDestroy:             testAccCheckKatapultFSVDestroy(tt),
// 		Steps: []resource.TestStep{
// 			{
// 				Config: undent.Stringf(`
// 					resource "katapult_file_storage_volume" "data" {
// 						name = "%s"
// 					}`,
// 					name,
// 				),
// 				Check: resource.ComposeAggregateTestCheckFunc(
// 					testAccCheckKatapultFileStorageVolumeAttrs(
// 						tt, "katapult_file_storage_volume.data",
// 					),
// 					resource.TestCheckResourceAttr(
// 						"katapult_file_storage_volume.data",
// 						"associations.#", "0",
// 					),
// 				),
// 			},
// 			{
// 				Config: undent.Stringf(`
// 					resource "katapult_ip" "web" {}

// 					resource "katapult_virtual_machine" "web" {
// 						hostname = "%s-web"
// 						package       = "rock-3"
// 						disk_template = "ubuntu-18-04"
// 						disk_template_options = {
// 							install_agent = true
// 						}
// 						ip_address_ids = [katapult_ip.web.id]
// 					}

// 					resource "katapult_file_storage_volume" "data" {
// 						name = "%s"
// 						associations = [
// 							katapult_virtual_machine.web.id
// 						]
// 					}

// 					resource "katapult_file_storage_volume" "cache" {
// 						name = "%s-cache"
// 						associations = [
// 							katapult_virtual_machine.web.id
// 						]
// 					}`,
// 					name, name, name,
// 				),
// 				Check: resource.ComposeAggregateTestCheckFunc(
// 					testAccCheckKatapultFileStorageVolumeAttrs(
// 						tt, "katapult_file_storage_volume.data",
// 					),
// 					testAccCheckKatapultFileStorageVolumeAttrs(
// 						tt, "katapult_file_storage_volume.cache",
// 					),
// 				),
// 			},
// 			{
// 				Config: undent.Stringf(`
// 					resource "katapult_ip" "web" {}
// 					resource "katapult_virtual_machine" "web" {
// 						hostname = "%s-web"
// 						package       = "rock-3"
// 						disk_template = "ubuntu-18-04"
// 						disk_template_options = {
// 							install_agent = true
// 						}
// 						ip_address_ids = [katapult_ip.web.id]
// 					}

// 					resource "katapult_ip" "db" {}
// 					resource "katapult_virtual_machine" "db" {
// 						hostname = "%s-db"
// 						package       = "rock-3"
// 						disk_template = "ubuntu-18-04"
// 						disk_template_options = {
// 							install_agent = true
// 						}
// 						ip_address_ids = [katapult_ip.db.id]
// 					}

// 					resource "katapult_file_storage_volume" "data" {
// 						name = "%s"
// 						associations = [
// 							katapult_virtual_machine.web.id
// 						]
// 					}

// 					resource "katapult_file_storage_volume" "cache" {
// 						name = "%s-cache"
// 						associations = [
// 							katapult_virtual_machine.db.id
// 						]
// 					}`,
// 					name, name, name, name,
// 				),
// 				Check: resource.ComposeAggregateTestCheckFunc(
// 					testAccCheckKatapultFileStorageVolumeAttrs(
// 						tt, "katapult_file_storage_volume.data",
// 					),
// 					testAccCheckKatapultFileStorageVolumeAttrs(
// 						tt, "katapult_file_storage_volume.cache",
// 					),
// 				),
// 			},
// 			{
// 				Config: undent.Stringf(`
// 					resource "katapult_ip" "web" {}
// 					resource "katapult_virtual_machine" "web" {
// 						hostname = "%s-web"
// 						package       = "rock-3"
// 						disk_template = "ubuntu-18-04"
// 						disk_template_options = {
// 							install_agent = true
// 						}
// 						ip_address_ids = [katapult_ip.web.id]
// 					}

// 					resource "katapult_file_storage_volume" "data" {
// 						name = "%s"
// 						associations = [
// 							katapult_virtual_machine.web.id
// 						]
// 					}

// 					resource "katapult_file_storage_volume" "cache" {
// 						name = "%s-cache"
// 						associations = []
// 					}`,
// 					name, name, name,
// 				),
// 				Check: resource.ComposeAggregateTestCheckFunc(
// 					testAccCheckKatapultFileStorageVolumeAttrs(
// 						tt, "katapult_file_storage_volume.data",
// 					),
// 					testAccCheckKatapultFileStorageVolumeAttrs(
// 						tt, "katapult_file_storage_volume.cache",
// 					),
// 				),
// 			},
// 			{
// 				Config: undent.Stringf(`
// 					resource "katapult_file_storage_volume" "data" {
// 						name = "%s"
// 						associations = []
// 					}

// 					resource "katapult_file_storage_volume" "cache" {
// 						name = "%s-cache"
// 						associations = []
// 					}`,
// 					name, name,
// 				),
// 				Check: resource.ComposeAggregateTestCheckFunc(
// 					testAccCheckKatapultFileStorageVolumeAttrs(
// 						tt, "katapult_file_storage_volume.data",
// 					),
// 					testAccCheckKatapultFileStorageVolumeAttrs(
// 						tt, "katapult_file_storage_volume.cache",
// 					),
// 				),
// 			},
// 			{
// 				ResourceName:      "katapult_file_storage_volume.data",
// 				ImportState:       true,
// 				ImportStateVerify: true,
// 			},
// 			{
// 				ResourceName:      "katapult_file_storage_volume.cache",
// 				ImportState:       true,
// 				ImportStateVerify: true,
// 			},
// 		},
// 	})
// }

//
// Test Helpers
//

func testAccCheckKatapultFileStorageVolumeAttrs(
	tt *testTools,
	res string,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("resource not found: %s", res)
		}

		var err error
		fsv, _, err := tt.Meta.Core.FileStorageVolumes.GetByID(
			tt.Ctx, rs.Primary.ID,
		)
		if err != nil {
			return err
		}

		tfs := []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(res, "id", fsv.ID),
			resource.TestCheckResourceAttr(res, "name", fsv.Name),
			resource.TestCheckResourceAttr(
				res, "associations.#", strconv.Itoa(len(fsv.Associations)),
			),
			resource.TestCheckResourceAttr(
				res, "nfs_location", fsv.NFSLocation,
			),
		}

		for _, assoc := range fsv.Associations {
			tfs = append(tfs, resource.TestCheckTypeSetElemAttr(
				res, "associations.*", assoc,
			))
		}

		return resource.ComposeAggregateTestCheckFunc(tfs...)(s)
	}
}

func testAccCheckKatapultFSVDestroy(
	tt *testTools,
) resource.TestCheckFunc {
	m := tt.Meta

	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "katapult_file_storage_volume" {
				continue
			}

			_, _, err := m.Core.FileStorageVolumes.GetByID(
				tt.Ctx, rs.Primary.ID,
			)
			if !errors.Is(err, katapult.ErrNotFound) {
				if err != nil {
					return err
				}

				return fmt.Errorf(
					"katapult_file_storage_volume %s was not destroyed",
					rs.Primary.ID,
				)
			}

			_, _, err = m.Core.TrashObjects.GetByObjectID(
				tt.Ctx, rs.Primary.ID,
			)
			if !errors.Is(err, katapult.ErrNotFound) {
				if err != nil {
					return err
				}

				return fmt.Errorf(
					"katapult_file_storage_volume %s was deleted, but not "+
						"purged from trash",
					rs.Primary.ID,
				)
			}
		}

		return nil
	}
}
