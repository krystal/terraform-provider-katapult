# Minimal
resource "katapult_security_group" "minimal" {
  name = "minimal"
}

# Practical
resource "katapult_virtual_machine_group" "web" {
  name = "web"
}

resource "katapult_virtual_machine_group" "monitoring" {
  name = "monitoring"
}

resource "katapult_security_group" "practical" {
  name = "practical"

  # Apply security group to all virtual machines in the web group.
  associations = [
    katapult_virtual_machine_group.web.id,
  ]

  # Allow all outbound traffic.
  allow_all_outbound = true

  inbound_rules = [
    # Deny SSH from the carrier-grade NAT range. Katapult evaluates deny rules
    # before allow rules, regardless of list order.
    {
      action   = "deny"
      protocol = "TCP"
      ports    = "22"
      targets  = ["100.64.0.0/10"]
      notes    = "Deny SSH from CGNAT"
    },
    # Action defaults to allow, so SSH is allowed from everywhere else.
    {
      protocol = "TCP"
      ports    = "22"
      targets  = ["all:ipv4", "all:ipv6"]
      notes    = "SSH"
    },
    {
      protocol = "TCP"
      ports    = "80,443"
      targets  = ["all:ipv4", "all:ipv6"]
      notes    = "HTTP & HTTPS"
    },
    {
      protocol = "UDP"
      ports    = "443"
      targets  = ["all:ipv4", "all:ipv6"]
      notes    = "QUIC"
    },
    # Allow inbound ICMP traffic from virtual machines in the monitoring group.
    {
      protocol = "ICMP"
      targets  = [katapult_virtual_machine_group.monitoring.id]
      notes    = "ping"
    },
  ]
}

# Dynamic Rules
locals {
  my_rules = {
    inbound = [
      {
        protocol = "TCP"
        ports    = "22"
        targets  = ["all:ipv4", "all:ipv6"]
        notes    = "SSH"
      },
      {
        protocol = "TCP"
        ports    = "80,443"
        targets  = ["all:ipv4", "all:ipv6"]
        notes    = "HTTP & HTTPS"
      },
      {
        protocol = "UDP"
        ports    = "443"
        targets  = ["all:ipv4", "all:ipv6"]
        notes    = "QUIC"
      },
    ]
    outbound = []
  }
}

resource "katapult_security_group" "dynamic" {
  name = "dynamic"

  # Set allow all attributes based on if any rules are defined.
  allow_all_inbound  = length(local.my_rules.inbound) > 0 ? false : true
  allow_all_outbound = length(local.my_rules.outbound) > 0 ? false : true

  inbound_rules  = local.my_rules.inbound
  outbound_rules = local.my_rules.outbound
}
