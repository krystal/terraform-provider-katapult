resource "katapult_self_signed_certificate" "internal" {
  name = "app.internal.example.com"
}

action "katapult_certificate_reset_token" "internal" {
  config {
    certificate_id = katapult_self_signed_certificate.internal.id
  }
}

# Rotate the bearer token embedded in `certificate_api_url` whenever this
# trigger resource is created or replaced. Change `input` to rotate again. The
# certificate resource's `certificate_api_url` refreshes on the next plan.
resource "terraform_data" "reset_token" {
  input = "2026-09-04"

  lifecycle {
    action_trigger {
      events  = [after_create, after_update]
      actions = [action.katapult_certificate_reset_token.internal]
    }
  }
}
