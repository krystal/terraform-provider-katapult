# Issue a self-signed certificate for an internal hostname. Katapult issues an
# RSA 4096 certificate valid for one year and re-issues it automatically a
# month before it expires.
resource "katapult_self_signed_certificate" "internal" {
  name             = "app.internal.example.com"
  additional_names = ["api.internal.example.com"]

  # Argument changes replace the certificate, and a certificate cannot be
  # deleted while a load balancer rule references it, so create the
  # replacement before destroying the old one.
  lifecycle {
    create_before_destroy = true
  }
}

# Attach the certificate to an HTTPS load balancer rule.
resource "katapult_load_balancer" "internal" {
  name = "internal"
}

resource "katapult_load_balancer_rule" "https" {
  load_balancer_id = katapult_load_balancer.internal.id
  protocol         = "HTTPS"
  listen_port      = 443
  destination_port = 8080
  certificate_ids  = [katapult_self_signed_certificate.internal.id]
}
