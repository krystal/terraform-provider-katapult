# Get security group by ID with rules.
data "katapult_security_group" "web" {
  id = "sg_O3NQJHXsgan6vO1V"
}

# Get security group by ID without rules.
data "katapult_security_group" "web-plain" {
  id            = "sg_O3NQJHXsgan6vO1V"
  include_rules = false
}
