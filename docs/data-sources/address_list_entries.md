---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "katapult_address_list_entries Data Source - terraform-provider-katapult"
subcategory: ""
description: |-
  
---

# katapult_address_list_entries (Data Source)



## Example Usage

```terraform
# Get address list entries by specific address list ID
data "katapult_address_lists" "public" {
  address_list_id = "adlst_wUevj8cYSRBfjYTA"
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `address_list_id` (String)

### Read-Only

- `entries` (Attributes Set) (see [below for nested schema](#nestedatt--entries))

<a id="nestedatt--entries"></a>
### Nested Schema for `entries`

Read-Only:

- `address` (String)
- `id` (String)
- `name` (String)
