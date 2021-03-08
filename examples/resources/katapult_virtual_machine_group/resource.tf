# Create a segregated virtual machine group
resource "katapult_virtual_machine_group" "web-1" {
  name = "vm group"
}

# Create a non-segregated virtual machine group
resource "katapult_virtual_machine_group" "web-1" {
  name      = "vm group"
  segregate = false
}
