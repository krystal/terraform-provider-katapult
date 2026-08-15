# List the disk I/O profiles available to the provider organization.
data "katapult_disk_io_profiles" "available" {}

output "disk_io_profiles" {
  value = {
    for profile in data.katapult_disk_io_profiles.available.profiles :
    profile.permalink => {
      speed_in_mb = profile.speed_in_mb
      iops        = profile.iops
    }
  }
}
