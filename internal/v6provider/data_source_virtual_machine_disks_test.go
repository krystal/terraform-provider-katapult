package v6provider

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jimeh/undent"
	core "github.com/krystal/go-katapult/next/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSortVMDiskAttachmentsByIDPreservesPairing(t *testing.T) {
	t.Parallel()

	attachments := []core.GetVirtualMachineDisks200ResponseDisks{
		{Disk: &core.GetVirtualMachineDisksPartDisk{Id: ptr("disk_z")}, Boot: ptr(true)},
		{Disk: nil, Boot: ptr(false)},
		{Disk: &core.GetVirtualMachineDisksPartDisk{Id: ptr("disk_a")}, AttachOnBoot: ptr(false)},
	}
	sortVMDiskAttachmentsByID(attachments)

	require.Len(t, attachments, 3)
	assert.Equal(t, "disk_a", *attachments[0].Disk.Id)
	assert.Equal(t, false, *attachments[0].AttachOnBoot)
	assert.Equal(t, "disk_z", *attachments[1].Disk.Id)
	assert.Equal(t, true, *attachments[1].Boot)
	assert.Nil(t, attachments[2].Disk)

	client := newVirtualMachineTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("disk[id]")
		writeTestJSON(w, http.StatusOK, fmt.Sprintf(`{"disk":{"id":%q}}`, id))
	})
	disks, err := fetchDiskDetailsForAttachments(
		context.Background(), &Meta{Core: client}, attachments,
	)
	require.NoError(t, err)
	require.Len(t, disks, 3)
	assert.Equal(t, "disk_a", *disks[0].Id)
	assert.Equal(t, "disk_z", *disks[1].Id)
	assert.Nil(t, disks[2])
}

func TestAccKatapultDataSourceVirtualMachineDisks_basic(t *testing.T) {
	tt := newTestTools(t)

	diskName := tt.ResourceName("disk")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: tt.ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckKatapultVirtualMachineDestroy(tt),
			testAccCheckKatapultDiskDestroy(tt),
			testAccCheckKatapultIPDestroy(tt),
		),
		Steps: []resource.TestStep{
			{
				Config: undent.Stringf(`
					resource "katapult_ip" "web" {}

					resource "katapult_disk" "data" {
					  name       = "%s"
					  size_in_gb = 20
					}

					resource "katapult_virtual_machine" "base" {
					  package       = "rock-3"
					  disk_template = "ubuntu-18-04"
					  disk_template_options = {
					    install_agent = true
					  }
					  ip_address_ids = [katapult_ip.web.id]
					}

					resource "katapult_disk_assignment" "data" {
					  virtual_machine_id = katapult_virtual_machine.base.id
					  disk_id            = katapult_disk.data.id
					}

					data "katapult_virtual_machine_disks" "all" {
					  virtual_machine_id = katapult_virtual_machine.base.id
					  depends_on         = [katapult_disk_assignment.data]
					}`, diskName,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.katapult_virtual_machine_disks.all",
						"disks.#", "2",
					),
					testAccCheckDataSourceDiskMatch(
						"data.katapult_virtual_machine_disks.all",
						"katapult_disk.data",
						false,
					),
					testAccCheckDataSourceHasOneBootDisk(
						"data.katapult_virtual_machine_disks.all",
					),
				),
			},
		},
	})
}

// testAccCheckDataSourceDiskMatch finds a disk in the data source matching the
// referenced standalone disk resource by ID, asserting boot equals expectBoot.
func testAccCheckDataSourceDiskMatch(
	dsAddr, diskAddr string,
	expectBoot bool,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ds, ok := s.RootModule().Resources[dsAddr]
		if !ok {
			return fmt.Errorf("not found: %s", dsAddr)
		}
		disk, ok := s.RootModule().Resources[diskAddr]
		if !ok {
			return fmt.Errorf("not found: %s", diskAddr)
		}
		want := disk.Primary.ID

		count, _ := strconv.Atoi(ds.Primary.Attributes["disks.#"])
		for i := 0; i < count; i++ {
			id := ds.Primary.Attributes[fmt.Sprintf("disks.%d.id", i)]
			if id != want {
				continue
			}
			boot := ds.Primary.Attributes[fmt.Sprintf("disks.%d.boot", i)] == "true"
			if boot != expectBoot {
				return fmt.Errorf(
					"disk %s: expected boot=%v, got boot=%v",
					id, expectBoot, boot,
				)
			}
			return nil
		}
		return fmt.Errorf(
			"disk %s not present in %s.disks",
			want, dsAddr,
		)
	}
}

func testAccCheckDataSourceHasOneBootDisk(
	dsAddr string,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ds, ok := s.RootModule().Resources[dsAddr]
		if !ok {
			return fmt.Errorf("not found: %s", dsAddr)
		}

		count, _ := strconv.Atoi(ds.Primary.Attributes["disks.#"])
		boots := 0
		for i := 0; i < count; i++ {
			if ds.Primary.Attributes[fmt.Sprintf("disks.%d.boot", i)] == "true" {
				boots++
			}
		}
		if boots != 1 {
			return fmt.Errorf(
				"expected exactly 1 boot disk, got %d", boots,
			)
		}
		return nil
	}
}
