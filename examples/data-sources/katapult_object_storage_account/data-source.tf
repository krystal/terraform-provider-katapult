data "katapult_object_storage_account" "main" {
  region = "uk-lon-1"
}

# Attach a bucket to the existing account without managing the account
# resource in this configuration.
resource "katapult_object_storage_bucket" "assets" {
  name                      = "my-org-assets"
  object_storage_account_id = data.katapult_object_storage_account.main.id
}
