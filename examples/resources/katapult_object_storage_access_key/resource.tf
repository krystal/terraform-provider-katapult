# Required entrypoint: one account per (organization, region).
resource "katapult_object_storage_account" "main" {
  region = "uk-lon-1"
}

# Minimal — key with no global permissions
resource "katapult_object_storage_access_key" "app" {
  name                      = "app-server"
  object_storage_account_id = katapult_object_storage_account.main.id
}

# Key with cluster-wide read/write access
resource "katapult_object_storage_access_key" "admin" {
  name                      = "ci-admin"
  object_storage_account_id = katapult_object_storage_account.main.id

  all_buckets_read  = true
  all_objects_read  = true
  all_objects_write = true
}

# Use the credentials to configure an object storage client
resource "katapult_object_storage_access_key" "backup" {
  name                      = "backup-agent"
  object_storage_account_id = katapult_object_storage_account.main.id
}

resource "katapult_object_storage_bucket" "backups" {
  name                      = "my-org-backups"
  object_storage_account_id = katapult_object_storage_account.main.id
  read_key_ids              = [katapult_object_storage_access_key.backup.id]
  write_key_ids             = [katapult_object_storage_access_key.backup.id]
}

output "backup_access_key_id" {
  value = katapult_object_storage_access_key.backup.access_key_id
}

output "backup_secret_access_key" {
  value     = katapult_object_storage_access_key.backup.secret_access_key
  sensitive = true
}

output "backup_server_url" {
  value = katapult_object_storage_access_key.backup.server_url
}
