# List every disk in the provider organization.
data "katapult_disks" "all" {}

# Build an ID-keyed inventory without relying on list positions.
locals {
  disks_by_id = {
    for disk in data.katapult_disks.all.disks : disk.id => disk
  }
}
