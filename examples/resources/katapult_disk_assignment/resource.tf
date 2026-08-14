resource "katapult_disk_assignment" "data" {
  virtual_machine_id = katapult_virtual_machine.web.id
  disk_id            = katapult_disk.data.id

  # Defaults to true. For a stopped VM this enables attach-on-boot while the
  # physical attachment correctly remains detached until the next start.
  attached = true
}
