# Get all security group rules for a security group.
data "katapult_security_group_rules" "web" {
  security_group_id = "sg_O3NQJHXsgan6vO1V"
}
