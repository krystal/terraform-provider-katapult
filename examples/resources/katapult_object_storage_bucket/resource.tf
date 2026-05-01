# Minimal — private bucket
resource "katapult_object_storage_bucket" "assets" {
  name   = "my-org-assets"
  region = "uk-lon-1"
}

# Public static site
resource "katapult_object_storage_bucket" "site" {
  name   = "my-org-static-site"
  region = "uk-lon-1"

  serve_static_site = true
  static_site_index = "index.html"
  static_site_error = ".html" # 404s redirect to /404.html

  public_list = true
  public_read = true
}

# Bucket with per-key access control
resource "katapult_object_storage_access_key" "app" {
  name   = "app-server"
  region = "uk-lon-1"
}

resource "katapult_object_storage_bucket" "uploads" {
  name   = "my-org-uploads"
  region = "uk-lon-1"

  # Grant the app key read and write access.
  read_key_ids  = [katapult_object_storage_access_key.app.id]
  write_key_ids = [katapult_object_storage_access_key.app.id]
}
