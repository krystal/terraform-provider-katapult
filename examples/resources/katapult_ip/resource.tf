# Create a IPv4 address
resource "katapult_ip" "web-1" {}

# Create a IPv4 address
resource "katapult_ip" "web-1-v6" {
  version = 6
}

resource "katapult_ip" "primary-db" {
  vip   = true
  label = "database"
}
