---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "katapult_virtual_machine_packages Data Source - terraform-provider-katapult"
subcategory: ""
description: |-
  Fetch details of all Virtual Machine Packages
---

# katapult_virtual_machine_packages (Data Source)

Fetch details of all Virtual Machine Packages

## Example Usage

```terraform
# Get all virtual machine packages
data "katapult_virtual_machine_packages" "all" {}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Read-Only

- `id` (String) Always set to `all`.
- `packages` (List of Object) (see [below for nested schema](#nestedatt--packages))

<a id="nestedatt--packages"></a>
### Nested Schema for `packages`

Read-Only:

- `cpu_cores` (Number)
- `id` (String)
- `ipv4_addresses` (Number)
- `memory_in_gb` (Number)
- `name` (String)
- `permalink` (String)
- `privacy` (String)
- `storage_in_gb` (Number)
