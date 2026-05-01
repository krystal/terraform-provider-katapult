# Minimal — key with no global permissions
resource "katapult_object_storage_access_key" "app" {
  name   = "app-server"
  region = "uk-lon-1"
}

# Key with cluster-wide read/write access
resource "katapult_object_storage_access_key" "admin" {
  name   = "ci-admin"
  region = "uk-lon-1"

  all_buckets_read  = true
  all_objects_read  = true
  all_objects_write = true
}

# Use the credentials to configure an S3 client (e.g. AWS provider)
resource "katapult_object_storage_access_key" "backup" {
  name   = "backup-agent"
  region = "uk-lon-1"
}

resource "katapult_object_storage_bucket" "backups" {
  name          = "my-org-backups"
  region        = "uk-lon-1"
  read_key_ids  = [katapult_object_storage_access_key.backup.id]
  write_key_ids = [katapult_object_storage_access_key.backup.id]
}

output "backup_s3_access_key_id" {
  value = katapult_object_storage_access_key.backup.s3_access_key_id
}

output "backup_s3_secret_access_key" {
  value     = katapult_object_storage_access_key.backup.s3_secret_access_key
  sensitive = true
}

output "backup_server_url" {
  value = katapult_object_storage_access_key.backup.server_url
}
