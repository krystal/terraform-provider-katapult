resource "katapult_disk" "data" {
  name                = "web-data"
  size_in_gb          = 200
  initial_file_system = "ext4"

  # Offline is the default and includes filesystem-aware resizing. Detach the
  # disk (or stop its VM) before changing size. ext4 supports offline growth
  # and shrink. Explicit online growth leaves guest partition/filesystem
  # expansion to the operator.
  resize_method = "offline"

  # Growth normally completes quickly, including offline growth. Offline shrink
  # can take much longer because Katapult must shrink the filesystem and
  # partition before reducing the disk. The default update timeout is 2h; this
  # allows additional time for large shrink operations and is not the expected
  # duration of an ordinary resize.
  timeouts {
    update = "4h"
  }
}
