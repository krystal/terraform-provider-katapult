data "katapult_object_storage_account" "main" {
  region = "uk-lon-1"
}

data "katapult_object_storage_bucket" "assets" {
  name   = "my-org-assets"
  region = data.katapult_object_storage_account.main.region
}
