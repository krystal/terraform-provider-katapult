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

  group_id = katapult_virtual_machine_group.web.id
  tags     = ["web", "public"]

  package       = "rock-3"
  disk_template = "templates/ubuntu-20-04"
  disk_template_options = {
    install_agent = true
  }

  ip_address_ids = [
    katapult_ip.web-2.id,
    katapult_ip.web-2-internal.id
  ]

  # Use katapult_network_speed_profiles data source to get list of available
  # profiles.
  network_speed_profile = "1gbps"
}
