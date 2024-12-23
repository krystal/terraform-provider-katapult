package v6provider

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/next/core"
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

	//nolint:lll // generated type names are long
	toDelete := []core.GetOrganizationFileStorageVolumes200ResponseFileStorageVolumes{}
	totalPages := 2
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		res, err := m.Core.GetOrganizationFileStorageVolumesWithResponse(ctx,
			&core.GetOrganizationFileStorageVolumesParams{
				OrganizationSubDomain: &m.confOrganization,
				Page:                  &pageNum,
			})
		if err != nil {
			return err
		}

		totalPages, _ = res.JSON200.Pagination.TotalPages.Get()

		for _, fsv := range res.JSON200.FileStorageVolumes {
			if strings.HasPrefix(*fsv.Name, testAccResourceNamePrefix) {
				toDelete = append(toDelete, fsv)
			}
		}
	}

	for _, fsv := range toDelete {
		m.Logger.Info("deleting file storage volume",
			"id", fsv.Id, "name", fsv.Name,
		)

		res, err := m.Core.DeleteFileStorageVolumeWithResponse(ctx,
			core.DeleteFileStorageVolumeJSONRequestBody{
				FileStorageVolume: core.FileStorageVolumeLookup{
					Id: fsv.Id,
				},
			})
		if err != nil {
			return err
		}

		m.Logger.Info("purging file storage volume",
			"id", fsv.Id, "name", fsv.Name,
		)

		_, err = m.Core.DeleteTrashObjectWithResponse(ctx,
			core.DeleteTrashObjectJSONRequestBody{
				TrashObject: core.TrashObjectLookup{
					Id: res.JSON200.TrashObject.Id,
				},
			})
		if err != nil {
			return err
		}

		waiter := &retry.StateChangeConf{
			Pending: []string{"exists"},
			Target:  []string{"not_found"},
			Refresh: func() (interface{}, string, error) {
				_, e := m.Core.GetTrashObjectWithResponse(ctx,
					&core.GetTrashObjectParams{
						TrashObjectId: res.JSON200.TrashObject.Id,
					})

				if e != nil && errors.Is(e, core.ErrNotFound) {
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

func TestAccKatapultFileStorageVolume_associations(t *testing.T) {
	tt := newTestTools(t)

	name := strings.ToLower(tt.ResourceName())

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultFSVDestroy(tt),
		Steps: []resource.TestStep{
			// Associate data volume with web VM.
			{
				Config: undent.Stringf(`
					resource "katapult_ip" "web" {}

					resource "katapult_virtual_machine" "web" {
						hostname = "%s-web"
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_ip.web.id]
					}

					resource "katapult_file_storage_volume" "data" {
						name = "%s"
						associations = [
							katapult_virtual_machine.web.id
						]
					}

					resource "katapult_file_storage_volume" "cache" {
						name = "%s-cache"
						// Ensure cache volume is created after data volume for
						// the sake of testing with VCR replay mode.
						depends_on = [katapult_file_storage_volume.data]
					}`,
					name, name, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultFileStorageVolumeAttrs(
						tt, "katapult_file_storage_volume.data",
					),
					resource.TestCheckResourceAttr(
						"katapult_file_storage_volume.data",
						"associations.#", "1",
					),
					resource.TestCheckResourceAttr(
						"katapult_file_storage_volume.cache",
						"associations.#", "0",
					),
				),
			},
			// Associate cache volume with web VM.
			{
				Config: undent.Stringf(`
					resource "katapult_ip" "web" {}

					resource "katapult_virtual_machine" "web" {
						hostname = "%s-web"
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_ip.web.id]
					}

					resource "katapult_file_storage_volume" "data" {
						name = "%s"
						associations = [
							katapult_virtual_machine.web.id
						]
					}

					resource "katapult_file_storage_volume" "cache" {
						name = "%s-cache"
						associations = [
							katapult_virtual_machine.web.id
						]
					}`,
					name, name, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultFileStorageVolumeAttrs(
						tt, "katapult_file_storage_volume.data",
					),
					testAccCheckKatapultFileStorageVolumeAttrs(
						tt, "katapult_file_storage_volume.cache",
					),
					resource.TestCheckResourceAttr(
						"katapult_file_storage_volume.cache",
						"associations.#", "1",
					),
				),
			},
			// Associate cache volume with db VM.
			{
				Config: undent.Stringf(`
					resource "katapult_ip" "web" {}
					resource "katapult_virtual_machine" "web" {
						hostname = "%s-web"
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_ip.web.id]
					}

					resource "katapult_ip" "db" {}
					resource "katapult_virtual_machine" "db" {
						hostname = "%s-db"
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_ip.db.id]
					}

					resource "katapult_file_storage_volume" "data" {
						name = "%s"
						associations = [
							katapult_virtual_machine.web.id
						]
					}

					resource "katapult_file_storage_volume" "cache" {
						name = "%s-cache"
						associations = [
							katapult_virtual_machine.web.id,
							katapult_virtual_machine.db.id
						]
					}`,
					name, name, name, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultFileStorageVolumeAttrs(
						tt, "katapult_file_storage_volume.data",
					),
					testAccCheckKatapultFileStorageVolumeAttrs(
						tt, "katapult_file_storage_volume.cache",
					),
					resource.TestCheckResourceAttr(
						"katapult_file_storage_volume.cache",
						"associations.#", "2",
					),
				),
			},
			// disasssociate cache volume from web VM.
			{
				Config: undent.Stringf(`
					resource "katapult_ip" "web" {}
					resource "katapult_virtual_machine" "web" {
						hostname = "%s-web"
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_ip.web.id]
					}

					resource "katapult_ip" "db" {}
					resource "katapult_virtual_machine" "db" {
						hostname = "%s-db"
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_ip.db.id]
					}

					resource "katapult_file_storage_volume" "data" {
						name = "%s"
						associations = [
							katapult_virtual_machine.web.id
						]
					}

					resource "katapult_file_storage_volume" "cache" {
						name = "%s-cache"
						associations = [
							katapult_virtual_machine.db.id
						]
					}`,
					name, name, name, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultFileStorageVolumeAttrs(
						tt, "katapult_file_storage_volume.data",
					),
					testAccCheckKatapultFileStorageVolumeAttrs(
						tt, "katapult_file_storage_volume.cache",
					),
					resource.TestCheckResourceAttr(
						"katapult_file_storage_volume.cache",
						"associations.#", "1",
					),
				),
			},
			// Disassociate cache volume by setting attributes to empty list.
			{
				Config: undent.Stringf(`
					resource "katapult_ip" "web" {}
					resource "katapult_virtual_machine" "web" {
						hostname = "%s-web"
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_ip.web.id]
					}

					resource "katapult_ip" "db" {}
					resource "katapult_virtual_machine" "db" {
						hostname = "%s-db"
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_ip.db.id]
					}

					resource "katapult_file_storage_volume" "data" {
						name = "%s"
						associations = [
							katapult_virtual_machine.web.id
						]
					}

					resource "katapult_file_storage_volume" "cache" {
						name = "%s-cache"
						associations = []
					}`,
					name, name, name, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultFileStorageVolumeAttrs(
						tt, "katapult_file_storage_volume.data",
					),
					testAccCheckKatapultFileStorageVolumeAttrs(
						tt, "katapult_file_storage_volume.cache",
					),
					resource.TestCheckResourceAttr(
						"katapult_file_storage_volume.cache",
						"associations.#", "0",
					),
				),
			},
			// Disassociate both volumes by completely removing associations
			// attribute in resource definitions.
			{
				Config: undent.Stringf(`
					resource "katapult_ip" "web" {}
					resource "katapult_virtual_machine" "web" {
						hostname = "%s-web"
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_ip.web.id]
					}

					resource "katapult_ip" "db" {}
					resource "katapult_virtual_machine" "db" {
						hostname = "%s-db"
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_ip.db.id]
					}

					resource "katapult_file_storage_volume" "data" {
						name = "%s"
					}

					resource "katapult_file_storage_volume" "cache" {
						name = "%s-cache"
					}`,
					name, name, name, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultFileStorageVolumeAttrs(
						tt, "katapult_file_storage_volume.data",
					),
					resource.TestCheckResourceAttr(
						"katapult_file_storage_volume.data",
						"associations.#", "0",
					),
					testAccCheckKatapultFileStorageVolumeAttrs(
						tt, "katapult_file_storage_volume.cache",
					),
					resource.TestCheckResourceAttr(
						"katapult_file_storage_volume.cache",
						"associations.#", "0",
					),
				),
			},
			{
				ResourceName:      "katapult_file_storage_volume.data",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "katapult_file_storage_volume.cache",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

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

		fsvRes, err := tt.Meta.Core.GetFileStorageVolumeWithResponse(tt.Ctx,
			&core.GetFileStorageVolumeParams{
				FileStorageVolumeId: &rs.Primary.ID,
			})
		if err != nil {
			return err
		}

		fsv := fsvRes.JSON200.FileStorageVolume

		NFSLocation, _ := fsv.NfsLocation.Get()

		tfs := []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(res, "id", *fsv.Id),
			resource.TestCheckResourceAttr(res, "name", *fsv.Name),
			resource.TestCheckResourceAttr(
				res, "associations.#", strconv.Itoa(len(*fsv.Associations)),
			),
			resource.TestCheckResourceAttr(
				res, "nfs_location", NFSLocation,
			),
		}

		for _, assoc := range *fsv.Associations {
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
	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "katapult_file_storage_volume" {
				continue
			}

			_, err := tt.Meta.Core.GetFileStorageVolumeWithResponse(tt.Ctx,
				&core.GetFileStorageVolumeParams{
					FileStorageVolumeId: &rs.Primary.ID,
				})

			if !errors.Is(err, core.ErrNotFound) {
				if err != nil {
					return err
				}

				return fmt.Errorf(
					"katapult_file_storage_volume %s was not destroyed",
					rs.Primary.ID,
				)
			}

			_, err = tt.Meta.Core.GetTrashObjectWithResponse(tt.Ctx,
				&core.GetTrashObjectParams{
					TrashObjectObjectId: &rs.Primary.ID,
				})
			if !errors.Is(err, core.ErrNotFound) {
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
