# Get specific virtual machine package by permalink
data "katapult_virtual_machine_package" "rock3" {
  permalink = "rock-3"
}

# Get specific virtual machine package by ID
data "katapult_virtual_machine_package" "rock3" {
  id = "vmpkg_Eh5LYVKScVHpj7sM"
}
