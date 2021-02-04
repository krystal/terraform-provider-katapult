resource "katapult_ip" "web-1" {}

resource "katapult_virtual_machine" "web-1" {
  package       = "rock-3"                 # permalink or ID
  disk_template = "templates/ubuntu-20-04" # permalink or ID
  disk_template_options = {
    install_agent = true # install_agent is required by some disk templates
  }
  ip_address_ids = [katapult_ip.web-1.id]
}
