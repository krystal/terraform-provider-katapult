---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "katapult_disk_templates Data Source - terraform-provider-katapult"
subcategory: ""
description: |-
  
---

# katapult_disk_templates (Data Source)



## Example Usage

```terraform
# Get all disk templates
data "katapult_disk_templates" "all" {}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Optional

- `include_universal` (Boolean) Include universal disk templates. Defaults to `true`.

### Read-Only

- `id` (String) Always set to provider organization value.
- `templates` (List of Object) (see [below for nested schema](#nestedatt--templates))

<a id="nestedatt--templates"></a>
### Nested Schema for `templates`

Read-Only:

- `description` (String)
- `id` (String)
- `name` (String)
- `os_family` (String)
- `permalink` (String)
- `template_version` (Number)
- `universal` (Boolean)
