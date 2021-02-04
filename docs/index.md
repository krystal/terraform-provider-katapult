---
page_title: "Katapult Provider"
subcategory: ""
description: |-
  The Katapult provider provides resources to interact with the Katapult.io API
---

# Katapult Provider

The Katapult provider provides resources to interact with the Katapult.io API.

## Example Usage

```terraform
provider "katapult" {
  api_key         = var.api_key         # or KATAPULT_API_KEY env var
  organization_id = var.organization_id # or KATAPULT_ORGANIZATION_ID env var
  data_center_id  = var.data_center_id  # or KATAPULT_DATA_CENTER_ID env var
}
```

### Required

- **api_key** (String)
- **data_center_id** (String)
- **organization_id** (String)
