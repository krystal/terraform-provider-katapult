# Create a segregated virtual machine group.
# Virtual Machines in this group will be placed on separate host machines
# where possible, improving availability.
resource "katapult_virtual_machine_group" "web" {
  name = "web"
}

# Create a non-segregated virtual machine group.
resource "katapult_virtual_machine_group" "batch" {
  name      = "batch"
  segregate = false
}
