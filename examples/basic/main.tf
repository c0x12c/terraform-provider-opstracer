terraform {
  required_providers {
    opstracer = {
      source = "c0x12c/opstracer"
    }
  }
}

# Reads OPSTRACER_TOKEN and OPSTRACER_API_URL from the environment.
provider "opstracer" {}

resource "opstracer_escalation_policy" "backend" {
  name        = "backend-primary"
  description = "Page the backend on-call, then the lead"

  levels = [
    {
      timeout_seconds = 600
      targets         = [{ type = "schedule", schedule_id = "<schedule-uuid>" }]
    },
    {
      targets = [{ type = "user", user_id = "<user-uuid>" }]
    },
  ]
}

resource "opstracer_service" "checkout" {
  name                 = "checkout-api"
  description          = "Checkout API"
  escalation_policy_id = opstracer_escalation_policy.backend.id
}
