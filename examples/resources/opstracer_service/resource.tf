resource "opstracer_service" "checkout" {
  name                 = "checkout-api"
  description          = "Checkout API"
  escalation_policy_id = opstracer_escalation_policy.backend.id
}
