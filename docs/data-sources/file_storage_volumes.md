---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "katapult_file_storage_volumes Data Source - terraform-provider-katapult"
subcategory: ""
description: |-
  Fetch all file storage volumes in the organization.
---

# katapult_file_storage_volumes (Data Source)

Fetch all file storage volumes in the organization.

## Example Usage

```terraform
# Get list of all file storage volumes.
data "katapult_file_storage_volumes" "all" {}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Read-Only

- `file_storage_volumes` (Attributes List) A list of file storage volumes. (see [below for nested schema](#nestedatt--file_storage_volumes))

<a id="nestedatt--file_storage_volumes"></a>
### Nested Schema for `file_storage_volumes`

Read-Only:

- `associations` (Set of String) The resource IDs which can access this file storage volume. Currently only accepts virtual machine IDs.
- `id` (String) The ID of the file storage volume.
- `name` (String) Unique name to help identify the volume. Must be unique within the organization.
- `nfs_location` (String) The NFS location indicating where to mount the volume from. This is where the volume must be mounted from inside of virtual machines referenced in `associations`.
- `size` (Number) The size of the volume in bytes.
