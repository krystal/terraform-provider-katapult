# Minimal
resource "katapult_ip" "web-1" {}

resource "katapult_virtual_machine" "web-1" {
  package       = "rock-3"
  disk_template = "templates/ubuntu-20-04"
  disk_template_options = {
    install_agent = true # required by some disk templates
  }
  ip_address_ids = [katapult_ip.web-1.id]
}

# Extensive
resource "katapult_ip" "web-2" {}
resource "katapult_ip" "web-2-internal" {}

resource "katapult_virtual_machine_group" "web" {
  name = "web-servers"
}

resource "katapult_virtual_machine" "base" {
  name        = "Web 2"
  hostname    = "web-2"
  description = "A web server."

  # Explicitly opt into ongoing power-state management. Set this to false to
  # gracefully shut down the VM and keep it stopped.
  powered_on = true

  group_id = katapult_virtual_machine_group.web.id
  tags     = ["web", "public"]

  package       = "rock-3"
  disk_template = "templates/ubuntu-20-04"
  disk_template_options = {
    install_agent = true
  }

  system_disk = {
    name       = "System Disk"
    size_in_gb = 20
  }

  ip_address_ids = [
    katapult_ip.web-2.id,
    katapult_ip.web-2-internal.id
  ]

  # Use katapult_network_speed_profiles data source to get list of available
  # profiles.
  network_speed_profile = "1gbps"
}

# Additional disks are independent objects and are assigned after the VM's
# first boot. The assignment is the sole owner of attach/detach lifecycle.
resource "katapult_disk" "web-2-data" {
  name       = "Data"
  size_in_gb = 100
}

resource "katapult_disk_assignment" "web-2-data" {
  virtual_machine_id = katapult_virtual_machine.base.id
  disk_id            = katapult_disk.web-2-data.id
}
