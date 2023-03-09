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

  # Allow inbound SSH, HTTP, HTTPS, and QUIC traffic from anywhere.
  inbound_rule {
    protocol = "TCP"
    ports    = "22"
    targets  = ["all:ipv4", "all:ipv6"]
    notes    = "SSH"
  }
  inbound_rule {
    protocol = "TCP"
    ports    = "80,433"
    targets  = ["all:ipv4", "all:ipv6"]
    notes    = "HTTP & HTTPS"
  }
  inbound_rule {
    protocol = "UDP"
    ports    = "443"
    targets  = ["all:ipv4", "all:ipv6"]
    notes    = "QUIC"
  }

  # Allow inbound ICMP traffic from virtual machines in the
  # monitoring group.
  inbound_rule {
    protocol = "ICMP"
    targets = [
      katapult_virtual_machine_group.monitoring.id
    ]
    notes = "ping"
  }
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
        ports    = "80,433"
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

  # Create inbound rules from local.my_rules.inbound values.
  dynamic "inbound_rule" {
    for_each = local.my_rules.inbound
    content {
      protocol = inbound_rule.value.protocol
      ports    = inbound_rule.value.ports
      targets  = inbound_rule.value.targets
      notes    = inbound_rule.value.notes
    }
  }

  # Create outbound rules from local.my_rules.outbound values.
  dynamic "outbound_rule" {
    for_each = local.my_rules.outbound
    content {
      protocol = outbound_rule.value.protocol
      ports    = outbound_rule.value.ports
      targets  = outbound_rule.value.targets
      notes    = outbound_rule.value.notes
    }
  }
}
