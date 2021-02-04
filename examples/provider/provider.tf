provider "katapult" {
  api_key         = var.api_key         # or KATAPULT_API_KEY env var
  organization_id = var.organization_id # or KATAPULT_ORGANIZATION_ID env var
  data_center_id  = var.data_center_id  # or KATAPULT_DATA_CENTER_ID env var
}
