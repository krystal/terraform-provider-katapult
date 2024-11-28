# Get network by ID.
data "katapult_network" "lon" {
  id = "netw_gVRkZdSKczfNg34P"
}

# Get network by permalink.
data "katapult_network" "nyc" {
  permalink = "us-nyc-01"
}

# Get default network for data center configured in provider.
data "katapult_network" "default" {}

# Get default network for specific data center.
data "katapult_network" "default-azp" {
  data_center_id = "dc_U3UVcwL8GKXJdsgw"
}
