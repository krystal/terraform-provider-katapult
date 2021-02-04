# Get data center specified in provider configuration
data "katapult_data_center" "default" {}

# Get specific data center by permalink
data "katapult_data_center" "lon" {
  permalink = "uk-lon-01"
}

# Get specific data center by ID
data "katapult_data_center" "lon" {
  id = "loc_UUhPmoCbpic6UX0Y"
}
