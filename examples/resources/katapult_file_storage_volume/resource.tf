# Minimal
resource "katapult_file_storage_volume" "cache" {
  name = "cache"
}

# Practical
resource "katapult_ip" "web" {}
resource "katapult_virtual_machine" "web" {
  hostname      = "web-1"
  package       = "rock-3"
  disk_template = "ubuntu-18-04"
  disk_template_options = {
    install_agent = true
  }
  ip_address_ids = [katapult_ip.db.id]
}

resource "katapult_file_storage_volume" "assets" {
  name = "assets"
  associations = [
    # Note: The still needs to be mounted on the VM using the
    # nfs_location attribute value.
    katapult_virtual_machine.web.id,
  ]
}
