package v6provider

import (
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
	"github.com/krystal/go-katapult/next/core"
)

func TestAccKatapultDataSourceFileStorageVolume_minimal(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultFSVDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_file_storage_volume" "my_vol" {
						name = "%s"
					}`,
					name,
				),
			},
			{
				Config: undent.Stringf(`
				resource "katapult_file_storage_volume" "my_vol" {
					name = "%s"
				}

				data "katapult_file_storage_volume" "my_vol" {
					id = katapult_file_storage_volume.my_vol.id
				}`,
					name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultDataSourceFileStorageVolumeAttrs(
						tt, "data.katapult_file_storage_volume.my_vol",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_file_storage_volume.my_vol", "name",
						"katapult_file_storage_volume.my_vol", "name",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_file_storage_volume.my_vol",
						"associations",
						"katapult_file_storage_volume.my_vol",
						"associations",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_file_storage_volume.my_vol",
						"nsf_location",
						"katapult_file_storage_volume.my_vol",
						"nsf_location",
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceFileStorageVolume_associations(t *testing.T) {
	tt := newTestTools(t)

	name := tt.ResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultFSVDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_legacy_ip" "web" {}
					resource "katapult_virtual_machine" "web" {
						hostname = "%s-web"
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_legacy_ip.web.id]
					}

					resource "katapult_file_storage_volume" "my_vol" {
						name = "%s"
						associations = [
							katapult_virtual_machine.web.id,
						]
					}

					data "katapult_file_storage_volume" "my_vol" {
						id = katapult_file_storage_volume.my_vol.id
					}`,
					name, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultDataSourceFileStorageVolumeAttrs(
						tt, "data.katapult_file_storage_volume.my_vol",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_file_storage_volume.my_vol", "name",
						"katapult_file_storage_volume.my_vol", "name",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_file_storage_volume.my_vol",
						"associations",
						"katapult_file_storage_volume.my_vol",
						"associations",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_file_storage_volume.my_vol",
						"nsf_location",
						"katapult_file_storage_volume.my_vol",
						"nsf_location",
					),
				),
			},
			{
				Config: undent.Stringf(`
					resource "katapult_legacy_ip" "web" {}
					resource "katapult_virtual_machine" "web" {
						hostname = "%s-web"
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_legacy_ip.web.id]
					}

					resource "katapult_legacy_ip" "db" {}
					resource "katapult_virtual_machine" "db" {
						hostname = "%s-db"
						package       = "rock-3"
						disk_template = "ubuntu-18-04"
						disk_template_options = {
							install_agent = true
						}
						ip_address_ids = [katapult_legacy_ip.db.id]
					}

					resource "katapult_file_storage_volume" "my_vol" {
						name = "%s"
						associations = [
							katapult_virtual_machine.web.id,
							katapult_virtual_machine.db.id,
						]
					}

					data "katapult_file_storage_volume" "my_vol" {
						id = katapult_file_storage_volume.my_vol.id
					}`,
					name, name, name,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckKatapultDataSourceFileStorageVolumeAttrs(
						tt, "data.katapult_file_storage_volume.my_vol",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_file_storage_volume.my_vol", "name",
						"katapult_file_storage_volume.my_vol", "name",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_file_storage_volume.my_vol",
						"associations",
						"katapult_file_storage_volume.my_vol",
						"associations",
					),
					resource.TestCheckResourceAttrPair(
						"data.katapult_file_storage_volume.my_vol",
						"nsf_location",
						"katapult_file_storage_volume.my_vol",
						"nsf_location",
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceFileStorageVolume_not_found(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultFSVDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					data "katapult_file_storage_volume" "my_vol" {
						id = "fsv_nopethisgonebye"
					}`,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						"katapult: not_found: file_storage_volume_not_found: " +
							"No file storage volume",
					),
				),
			},
		},
	})
}

func TestAccKatapultDataSourceFileStorageVolume_blank(t *testing.T) {
	tt := newTestTools(t)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy:             testAccCheckKatapultFSVDestroy(tt),
		Steps: []resource.TestStep{
			{
				Config: undent.String(`
					data "katapult_file_storage_volume" "my_vol" {

					}`,
				),
				ExpectError: regexp.MustCompile(
					regexp.QuoteMeta(
						`The argument "id" is required, but no definition ` +
							"was found.",
					),
				),
			},
		},
	})
}

func testAccCheckKatapultDataSourceFileStorageVolumeAttrs(
	tt *testTools,
	res string,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[res]
		if !ok {
			return fmt.Errorf("resource not found: %s", res)
		}

		var err error

		fsvRes, err := tt.Meta.Core.GetFileStorageVolumeWithResponse(tt.Ctx,
			&core.GetFileStorageVolumeParams{
				FileStorageVolumeId: &rs.Primary.ID,
			})
		if err != nil {
			return err
		}

		fsv := fsvRes.JSON200.FileStorageVolume

		Size, _ := fsv.Size.Get()
		NFSLocation, _ := fsv.NfsLocation.Get()

		tfs := []resource.TestCheckFunc{
			resource.TestCheckResourceAttr(res, "id", *fsv.Id),
			resource.TestCheckResourceAttr(res, "name", *fsv.Name),
			resource.TestCheckResourceAttr(
				res, "associations.#", strconv.Itoa(len(*fsv.Associations)),
			),
			resource.TestCheckResourceAttr(
				res, "size", strconv.FormatInt(int64(Size), 10),
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
