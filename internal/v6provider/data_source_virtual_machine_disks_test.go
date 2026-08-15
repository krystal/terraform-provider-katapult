package v6provider

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
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
		if r.URL.Path != "/disks/disk" {
			http.NotFound(w, r)
			return
		}
		id := r.URL.Query().Get("disk[id]")
		switch id {
		case "disk_a":
			writeTestJSON(w, http.StatusOK, `{"disk":{"id":"disk_a"}}`)
		case "disk_z":
			writeTestJSON(w, http.StatusOK, `{"disk":{"id":"disk_z"}}`)
		default:
			http.Error(w, "unexpected disk ID", http.StatusNotFound)
		}
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

func TestBuildDiskAttrObjectHandlesNullRelationships(t *testing.T) {
	t.Parallel()

	disk := core.GetDisk200ResponseDisk{Id: ptr("disk_test")}
	disk.BusType.SetNull()
	disk.IoProfile.SetNull()

	value, diags := buildDiskAttrObject(
		&disk, core.GetVirtualMachineDisks200ResponseDisks{}, "",
	)
	require.False(t, diags.HasError(), diags.Errors())
	object, ok := value.(types.Object)
	require.True(t, ok)
	require.True(t, object.Attributes()["bus_type"].IsNull())
	require.True(t, object.Attributes()["io_profile_id"].IsNull())
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
					checkVMDiskDataSourceEntry(
						"data.katapult_virtual_machine_disks.all",
						"katapult_disk.data",
						false,
						true,
						"attached",
						20,
					),
					checkVMDiskDataSourceSorted(
						"data.katapult_virtual_machine_disks.all",
					),
				),
			},
		},
	})
}

func checkVMDiskDataSourceEntry(
	dataSourceAddress, diskResourceAddress string,
	boot, attachOnBoot bool,
	attachmentState string,
	sizeInGB int,
) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		dataSource, ok := state.RootModule().Resources[dataSourceAddress]
		if !ok {
			return fmt.Errorf("not found: %s", dataSourceAddress)
		}
		disk, ok := state.RootModule().Resources[diskResourceAddress]
		if !ok {
			return fmt.Errorf("not found: %s", diskResourceAddress)
		}

		count, err := strconv.Atoi(dataSource.Primary.Attributes["disks.#"])
		if err != nil {
			return fmt.Errorf("reading %s disk count: %w", dataSourceAddress, err)
		}
		for i := range count {
			prefix := fmt.Sprintf("disks.%d.", i)
			if dataSource.Primary.Attributes[prefix+"id"] != disk.Primary.ID {
				continue
			}
			want := map[string]string{
				"name":             disk.Primary.Attributes["name"],
				"size_in_gb":       strconv.Itoa(sizeInGB),
				"boot":             strconv.FormatBool(boot),
				"attach_on_boot":   strconv.FormatBool(attachOnBoot),
				"attachment_state": attachmentState,
			}
			for attribute, expected := range want {
				if actual := dataSource.Primary.Attributes[prefix+attribute]; actual != expected {
					return fmt.Errorf(
						"%s%s: expected %q, got %q",
						prefix, attribute, expected, actual,
					)
				}
			}
			for _, attribute := range []string{"wwn", "state", "storage_speed"} {
				if dataSource.Primary.Attributes[prefix+attribute] == "" {
					return fmt.Errorf("%s%s is empty", prefix, attribute)
				}
			}
			return nil
		}
		return fmt.Errorf(
			"disk %s not present in %s.disks",
			disk.Primary.ID, dataSourceAddress,
		)
	}
}

func checkVMDiskDataSourceSorted(
	dataSourceAddress string,
) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		dataSource, ok := state.RootModule().Resources[dataSourceAddress]
		if !ok {
			return fmt.Errorf("not found: %s", dataSourceAddress)
		}
		count, err := strconv.Atoi(dataSource.Primary.Attributes["disks.#"])
		if err != nil {
			return fmt.Errorf("reading %s disk count: %w", dataSourceAddress, err)
		}
		previous := ""
		for i := range count {
			id := dataSource.Primary.Attributes[fmt.Sprintf("disks.%d.id", i)]
			if previous != "" && id < previous {
				return fmt.Errorf("disk IDs are not sorted: %q before %q", previous, id)
			}
			previous = id
		}
		return nil
	}
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
