# List every Virtual Machine in the provider organization.
data "katapult_virtual_machines" "all" {}

# Build an ID-keyed inventory without relying on list positions.
locals {
  virtual_machines_by_id = {
    for virtual_machine in data.katapult_virtual_machines.all.virtual_machines :
    virtual_machine.id => virtual_machine
  }
}
