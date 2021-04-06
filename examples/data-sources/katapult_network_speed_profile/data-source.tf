# Get network speed profile by permalink
data "katapult_network_speed_profile" "1gbps" {
  permalink = "1gbps"
}

# Get network speed profile by ID
data "katapult_network_speed_profile" "10gbps" {
  id = "nsp_FKZSFo5xhn9Pfr79"
}
