terraform {
  required_providers {
    katapult = {
      source = "katapult.io/katapult/katapult"
    }
  }
}

provider "katapult" {}

resource "katapult_ip" "east" {}
resource "katapult_ip" "west" {}

resource "katapult_virtual_machine" "db" {
  name          = "db 1"
  hostname      = "db-1"
  description   = "A db server."
  package       = "xsmall"
  disk_template = "ubuntu1804"
  disk_template_options = {
    install_agent = true
  }
  ip_address_ids = [
    katapult_ip.east.id,
    katapult_ip.west.id,
  ]
  tags = ["db", "public", "foo"]
}
