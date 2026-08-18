---
page_title: "Migrating Legacy Virtual Machine Disks"
description: |-
  Move deprecated Virtual Machine disk blocks to system_disk and first-class disk resources without recreating existing disks.
---

# Migrating Legacy Virtual Machine Disks

The deprecated `disk` blocks on
[`katapult_virtual_machine`](../resources/virtual_machine.md) are creation-time
inputs. Migrate the first block to the VM-owned `system_disk` attribute. Migrate
every additional disk to its own [`katapult_disk`](../resources/disk.md) and the
relationship to
[`katapult_disk_assignment`](../resources/disk_assignment.md).

This is a two-phase migration. Import the additional objects before removing
the legacy blocks so Terraform never treats an existing disk as a new object or
leaves it unmanaged.

## Inventory the existing disks

Refresh the current state and use
[`katapult_virtual_machine_disks`](../data-sources/virtual_machine_disks.md) to
inventory the exact disk IDs and relationships:

```terraform
data "katapult_virtual_machine_disks" "all" {
  virtual_machine_id = katapult_virtual_machine.base.id
}
```

```shell
terraform apply -refresh-only
terraform state show data.katapult_virtual_machine_disks.all
```

Record the disk whose `boot` value is `true` and every non-boot disk ID. Check
the name, size, `attach_on_boot`, and `attachment_state` values before changing
the configuration.

## Phase 1: import additional disks and assignments

Keep every existing `disk` block in the VM configuration. Alongside it, declare
one `katapult_disk` and one `katapult_disk_assignment` for each additional disk,
using values that match the existing object:

```terraform
resource "katapult_virtual_machine" "base" {
  # Existing VM attributes omitted.

  disk {
    name = "System"
    size = 20
  }

  disk {
    name = "Data"
    size = 100
  }
}

resource "katapult_disk" "data" {
  name       = "Data"
  size_in_gb = 100
}

resource "katapult_disk_assignment" "data" {
  virtual_machine_id = katapult_virtual_machine.base.id
  disk_id            = katapult_disk.data.id
  attached           = true
}
```

Import the existing additional disk and relationship:

```shell
terraform import katapult_disk.data disk_6Fz5U7mP5DM1V08Y
terraform import katapult_disk_assignment.data vm_BKBvUDVd6qt886QY/disk_6Fz5U7mP5DM1V08Y
```

The equivalent declarative import blocks are:

```terraform
import {
  to = katapult_disk.data
  id = "disk_6Fz5U7mP5DM1V08Y"
}

import {
  to = katapult_disk_assignment.data
  id = "vm_BKBvUDVd6qt886QY/disk_6Fz5U7mP5DM1V08Y"
}
```

Repeat this pair for every non-boot disk. Do not import the boot disk into a
`katapult_disk` or `katapult_disk_assignment`; it remains owned by the VM.

With declarative import blocks, run a plan before applying and require it to
show imports for the existing objects, not creation or replacement. With CLI
imports, inspect the imported state and require the following plan to contain
no creation or replacement. In either case, confirm that every additional disk
and assignment has the expected ID in Terraform state.

The API does not expose the filesystem type of an imported standalone disk.
Omit `initial_file_system` unless it is independently known. If configured, the
provider adopts the value once, but cannot verify it against the disk.

## Phase 2: replace the legacy blocks in configuration

After all additional disks and relationships are tracked, remove every
deprecated `disk` block and optionally add `system_disk` with values matching
the former first block:

```terraform
resource "katapult_virtual_machine" "base" {
  # Existing VM attributes omitted.

  system_disk = {
    name       = "System"
    size_in_gb = 20
  }
}

resource "katapult_disk" "data" {
  name       = "Data"
  size_in_gb = 100
}

resource "katapult_disk_assignment" "data" {
  virtual_machine_id = katapult_virtual_machine.base.id
  disk_id            = katapult_disk.data.id
  attached           = true
}
```

Terraform reports an in-place legacy-schema migration warning. The plan may
update VM state, but it must not replace the VM, recreate a disk, or change the
remote relationships. Apply it, then run `terraform plan` again and require an
empty plan.

## Attachment and deletion behavior

An assignment owns both the relationship and its attachment policy. With a
running VM, `attached = true` physically attaches the disk and enables attach
on boot. With a stopped VM, it enables attach on boot while the disk remains
physically detached until the VM starts. `attached = false` disables attach on
boot and detaches the disk.

Remove a `katapult_disk_assignment` before deleting either endpoint. The disk
resource refuses deletion while any assignment remains, and the VM resource
refuses deletion while non-boot assignments remain. References between the
three resources let Terraform order detach and unassign before endpoint
deletion.

VMs created by older provider versions may not have the exact legacy disk IDs
stored in private state. Such a VM cannot safely cascade-delete unknown
additional disks. Complete this migration before destroying it.

For adopting an entirely unmanaged VM and its related objects, see
[Importing Existing Virtual Machines](importing-virtual-machines.md).
