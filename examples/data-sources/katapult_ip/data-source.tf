# Get IP address by address
data "katapult_ip" "web-1" {
  address = "142.197.71.30"
}

# Get IP address by ID
data "katapult_ip" "web-2" {
  id = "ip_aQx7zQW7P2yBgjpw"
}
