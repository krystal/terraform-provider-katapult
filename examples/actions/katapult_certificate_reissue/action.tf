resource "katapult_self_signed_certificate" "internal" {
  name = "app.internal.example.com"
}

action "katapult_certificate_reissue" "internal" {
  config {
    certificate_id = katapult_self_signed_certificate.internal.id

    # Optional. How long to wait for the issuance task. Defaults to 10m.
    timeout = "15m"
  }
}

# Re-issue the certificate whenever this trigger resource is created or
# replaced. Change `input` to trigger another issuance. The certificate
# resource's `certificate`, `expires_at`, `last_issued_at`, and
# `certificate_api_url` refresh on the next plan.
resource "terraform_data" "reissue" {
  input = "2026-09-04"

  lifecycle {
    action_trigger {
      events  = [after_create, after_update]
      actions = [action.katapult_certificate_reissue.internal]
    }
  }
}
