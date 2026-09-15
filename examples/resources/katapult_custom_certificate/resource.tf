# Upload an existing certificate with its RSA private key and optional chain.
# Katapult derives name and additional_names from the certificate's common name
# and subject alternative names. The PEM inputs are kept in state exactly as
# configured.
resource "katapult_custom_certificate" "uploaded" {
  certificate = file("${path.module}/cert.pem")
  private_key = file("${path.module}/key.pem")
  chain       = file("${path.module}/chain.pem")

  # Argument changes replace the certificate, and a certificate cannot be
  # deleted while a load balancer rule references it, so create the
  # replacement before destroying the old one.
  lifecycle {
    create_before_destroy = true
  }
}

resource "katapult_load_balancer" "web" {
  name = "web"
}

resource "katapult_load_balancer_rule" "https" {
  load_balancer_id = katapult_load_balancer.web.id
  protocol         = "HTTPS"
  listen_port      = 443
  destination_port = 8080
  certificate_ids  = [katapult_custom_certificate.uploaded.id]
}
