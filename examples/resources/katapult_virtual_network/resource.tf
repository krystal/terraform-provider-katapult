# Create virtual network
resource "katapult_virtual_network" "backbone" {
  name = "Backbone"
}

# Create a virtual network in a specific Data Center.
data "katapult_data_center" "ams" {
  permalink = "nl-ams-01"
}

resource "katapult_virtual_network" "backbone-ams" {
  name           = "Backbone AMS"
  data_center_id = data.katapult_data_center.ams.id
}
