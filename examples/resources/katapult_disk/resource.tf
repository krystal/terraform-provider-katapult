resource "katapult_disk" "data" {
  name                = "web-data"
  size_in_gb          = 200
  initial_file_system = "ext4"

  # Offline is the default and includes filesystem-aware resizing. Detach the
  # disk (or stop its VM) before changing size. ext4 supports offline growth
  # and shrink. Explicit online growth leaves guest partition/filesystem
  # expansion to the operator.
  resize_method = "offline"

  timeouts {
    update = "4h"
  }
}
