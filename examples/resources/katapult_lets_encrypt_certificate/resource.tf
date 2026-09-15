resource "katapult_load_balancer" "web" {
  name = "web"
}

# With http authorization, Katapult answers the Let's Encrypt HTTP-01
# challenges through any port-80 listener on a load balancer in the same
# organization. DNS for every name must already point at this load balancer.
# The certificate does not need to be attached to a rule before it is issued.
resource "katapult_load_balancer_rule" "http" {
  load_balancer_id = katapult_load_balancer.web.id
  protocol         = "HTTP"
  listen_port      = 80
  destination_port = 8080
}

resource "katapult_lets_encrypt_certificate" "web" {
  name                 = "www.example.com"
  additional_names     = ["example.com"]
  authorization_method = "http"

  # The port-80 listener must exist before issuance starts.
  depends_on = [katapult_load_balancer_rule.http]

  # Argument changes replace the certificate, and a certificate cannot be
  # deleted while a load balancer rule references it, so create the
  # replacement before destroying the old one.
  lifecycle {
    create_before_destroy = true
  }
}

# One apply creates the port-80 listener, issues the certificate, and attaches
# it to the HTTPS rule.
resource "katapult_load_balancer_rule" "https" {
  load_balancer_id = katapult_load_balancer.web.id
  protocol         = "HTTPS"
  listen_port      = 443
  destination_port = 8080
  certificate_ids  = [katapult_lets_encrypt_certificate.web.id]
}

# With dns authorization, every name or a parent domain must be in a verified
# Katapult DNS zone. Katapult creates and removes the challenge records itself.
# Wildcard names require dns authorization.
resource "katapult_lets_encrypt_certificate" "wildcard" {
  name                 = "*.example.com"
  authorization_method = "dns"

  # Return once the certificate exists in the pending state instead of waiting
  # for issuance, for example when DNS is cut over outside Terraform. Katapult
  # keeps retrying issuance on its own schedule.
  wait_for_issuance = false
}
