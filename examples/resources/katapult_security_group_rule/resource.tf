# Security Group to create rules in.
resource "katapult_security_group" "web" {
  name = "web"

  # Prevent the security group resource itself to manage rules.
  external_rules = true
}

# Minimal - Allows no traffic as there are no targets.
resource "katapult_security_group_rule" "minimal" {
  security_group_id = katapult_security_group.web.id
  direction         = "inbound"
  protocol          = "tcp"
  targets           = []
}

# Allow incoming HTTP/HTTPS traffic from anywhere
resource "katapult_security_group_rule" "http" {
  security_group_id = katapult_security_group.web.id
  direction         = "inbound"
  protocol          = "tcp"
  ports             = "80,433"
  notes             = "Allow HTTP/HTTPS"
  targets = [
    "all:ipv4", # Allow any IPv4 address.
    "all:ipv6", # Allow any IPv6 address.
  ]
}

# Allow incoming SSH traffic from a specific IP and all virtual
# machines # in the "jumpbox" virtual machine group.
resource "katapult_virtual_machine_group" "jumpbox" {
  name = "jumpbox"
}

resource "katapult_security_group_rule" "ssh" {
  security_group_id = katapult_security_group.web.id
  direction         = "inbound"
  protocol          = "tcp"
  ports             = "22"
  notes             = "Allow SSH"
  targets = [
    "106.240.71.168",
    katapult_virtual_machine_group.jumpbox.id,
  ]
}

# Allow a range of ports from a CIDR block.
resource "katapult_security_group_rule" "range" {
  security_group_id = katapult_security_group.web.id
  direction         = "inbound"
  protocol          = "tcp"
  ports             = "3000-3999"
  notes             = "Allow custom range"
  targets           = ["152.15.204.0/24"]
}

# Allow all ports from a CIDR block.
resource "katapult_security_group_rule" "range_all_ports" {
  security_group_id = katapult_security_group.web.id
  direction         = "inbound"
  protocol          = "tcp"
  notes             = "Allow custom range"
  targets           = ["100.68.232.0/24"]
}

# Allow outgoing SMTP traffic to a specific IP.
resource "katapult_security_group_rule" "smtp" {
  security_group_id = katapult_security_group.web.id
  direction         = "outbound"
  protocol          = "tcp"
  ports             = "25"
  notes             = "Allow SMTP"
  targets           = ["195.66.84.24"]
}

# Allow outgoing HTTP and HTTPS traffic to anywhere.
resource "katapult_security_group_rule" "http_out" {
  security_group_id = katapult_security_group.web.id
  direction         = "outbound"
  protocol          = "tcp"
  ports             = "80,433"
  notes             = "Allow HTTP/HTTPS"
  targets           = ["all:ipv4", "all:ipv6"]
}
