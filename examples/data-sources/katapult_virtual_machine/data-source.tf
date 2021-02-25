# Get virtual machine by ID
data "katapult_virtual_machine" "web-1" {
  id = "vm_Ek42KaL1OrE7tkav"
}

# Get virtual machine by FQDN
data "katapult_virtual_machine" "web-1" {
  fqdn = "web-1.acme-labs.katapult.cloud"
}
