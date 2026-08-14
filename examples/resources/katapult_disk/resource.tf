resource "katapult_disk" "data" {
	name       = "web-data"
	size_in_gb = 200

	# Offline is the default and includes filesystem-aware resizing. Detach the
	# disk (or stop its VM) before changing size. Explicit online growth leaves
	# guest partition/filesystem expansion to the operator.
	resize_method = "offline"

	timeouts {
		update = "4h"
	}
}
