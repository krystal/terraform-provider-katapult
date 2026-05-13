# Only one of these may exist per (organization, region).
#
# If your organization already has object storage enabled via the Katapult
# dashboard, import the existing account rather than creating a new one:
#
#   terraform import katapult_object_storage_account.main uk-lon-1
resource "katapult_object_storage_account" "main" {
  region = "uk-lon-1"
}

output "account_provisioning_state" {
  value = katapult_object_storage_account.main.provisioning_state
}
