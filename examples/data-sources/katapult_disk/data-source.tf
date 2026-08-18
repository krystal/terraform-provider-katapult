# Look up an existing disk by ID.
data "katapult_disk" "database" {
  id = "disk_2YvW3Z8kQm5nL7pR"
}

# Inspect whether the disk is currently assigned and attached.
output "database_disk_assignment" {
  value = {
    virtual_machine_id = data.katapult_disk.database.virtual_machine_id
    attach_on_boot     = data.katapult_disk.database.attach_on_boot
    attachment_state   = data.katapult_disk.database.attachment_state
  }
}
