# Look up a disk I/O profile by permalink.
data "katapult_disk_io_profile" "fast" {
  permalink = "unrestricted"
}

# The profile ID can be used when creating a standalone disk.
resource "katapult_disk" "database" {
  name          = "database-data"
  size_in_gb    = 100
  io_profile_id = data.katapult_disk_io_profile.fast.id
}
