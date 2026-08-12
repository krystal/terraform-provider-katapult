# When Terraform manages the account, referencing its region establishes the
# required creation ordering. For an externally managed account, use the known
# region directly instead, for example: region = "uk-lon-1".
resource "katapult_object_storage_account" "main" {
  region = "uk-lon-1"
}

# Minimal — private bucket
resource "katapult_object_storage_bucket" "assets" {
  name   = "my-org-assets"
  region = katapult_object_storage_account.main.region
}

# Public static site
resource "katapult_object_storage_bucket" "site" {
  name   = "my-org-static-site"
  region = katapult_object_storage_account.main.region

  serve_static_site = true
  static_site_index = "index.html"
  static_site_error = ".html" # 404s redirect to /404.html

  public_list = true
  public_read = true
}

# Bucket with per-key access control
resource "katapult_object_storage_access_key" "app" {
  name   = "app-server"
  region = katapult_object_storage_account.main.region
}

resource "katapult_object_storage_bucket" "uploads" {
  name   = "my-org-uploads"
  region = katapult_object_storage_account.main.region

  # Grant the app key read and write access.
  read_key_ids  = [katapult_object_storage_access_key.app.id]
  write_key_ids = [katapult_object_storage_access_key.app.id]
}
