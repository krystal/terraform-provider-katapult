---
page_title: "Importing Existing Virtual Machines"
description: |-
  Adopt an existing Katapult Virtual Machine and its disks into Terraform without recreating them.
---

# Importing Existing Virtual Machines

Importing a [Katapult Virtual Machine](../resources/virtual_machine.md) adopts the
VM itself and discovers its VM-owned system disk. Additional disks and their
relationships are separate objects, so import each of those into
[`katapult_disk`](../resources/disk.md) and
[`katapult_disk_assignment`](../resources/disk_assignment.md) resources.

## Prepare the VM configuration

Write the `katapult_virtual_machine` configuration before importing. The
`package` and `ip_address_ids` attributes are required, and their values should
match the existing VM. Reference IP addresses, a VM group, and other related
objects that are already managed elsewhere in the same Terraform state rather
than importing them again.

Include the existing values of mutable relationships and settings that you want
Terraform to preserve. In particular, omitting a non-empty `description`,
`tags`, `group_id`, or `virtual_network_ids` value asks Terraform to remove it
on the adoption apply.

```terraform
resource "katapult_virtual_machine" "base" {
  name        = "Web 2"
  hostname    = "web-2"
  description = "A web server."

  package        = "rock-3"
  ip_address_ids = [
    katapult_ip.public.id,
    katapult_ip.internal.id,
  ]

  group_id            = katapult_virtual_machine_group.web.id
  tags                = ["public", "web"]
  virtual_network_ids = [katapult_virtual_network.private.id]
}
```

The `powered_on` attribute is optional. Omit it to observe the current power
state without managing it, or set it explicitly only when Terraform should
start and stop the VM.

## Import the VM

Import with the CLI:

```shell
terraform import katapult_virtual_machine.base vm_BKBvUDVd6qt886QY
```

Alternatively, use a declarative import block:

```terraform
import {
  to = katapult_virtual_machine.base
  id = "vm_BKBvUDVd6qt886QY"
}
```

Run `terraform plan` and inspect it before applying. The provider discovers the
system disk automatically and records it in `system_disk`; do not import that
boot disk as a standalone `katapult_disk`.

You can omit `system_disk` to accept the discovered name and size. To manage
those values, add a matching object after import:

```terraform
resource "katapult_virtual_machine" "base" {
  # Existing VM configuration omitted.

  system_disk = {
    name       = "System Disk"
    size_in_gb = 20
  }
}
```

## Creation-time disk-template values

The provider can validate `disk_template` against the installation recorded on
the system disk. Configure only the template that was actually used; choosing a
different template requires replacing the VM.

Katapult does not return `disk_template_options` for an existing VM. On the
first plan after import, the provider therefore trusts and adopts a configured
map once. Confirm those values independently before adding them. They describe
creation-time input only: adding `install_agent = "true"` after import does not
install the agent in the existing guest.

## Import additional disks and assignments

Use [`katapult_virtual_machine_disks`](../data-sources/virtual_machine_disks.md) to
inventory all disk relationships and identify the entry whose `boot` value is
`true`. For every non-boot entry, declare a disk and an assignment that match
the existing object and relationship:

```terraform
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

Import the two objects separately:

```shell
terraform import katapult_disk.data disk_6Fz5U7mP5DM1V08Y
terraform import katapult_disk_assignment.data vm_BKBvUDVd6qt886QY/disk_6Fz5U7mP5DM1V08Y
```

Or use declarative imports:

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

Set `attached` to the existing desired policy. For a stopped VM,
`attached = true` means attach on the next boot even though the observed
physical attachment remains detached.

The API does not expose the existing filesystem type of an imported standalone
disk. Omit `initial_file_system` unless it is independently known. The provider
allows an imported disk to adopt that value once without recreation, but it
cannot verify the value for you.

## Reconcile the first plan

The first post-import plan can contain an in-place VM update that adopts
creation-time configuration. Computed fields such as `fqdn`, `ip_addresses`,
`network_interfaces`, `state`, and parts of `system_disk` may temporarily show
as `(known after apply)` during that update. Applying can result in no remote
mutation and only settle Terraform state.

Review the plan carefully: it must not replace the VM or create disks that
already exist. After the adoption apply, run `terraform plan` again and require
an empty plan. If relationship changes repeat or the second plan is not empty,
stop and compare the configuration with the current Katapult objects before
continuing.

For a VM already managed through deprecated nested `disk` blocks, follow
[Migrating Legacy Virtual Machine Disks](migrating-legacy-virtual-machine-disks.md)
instead.
