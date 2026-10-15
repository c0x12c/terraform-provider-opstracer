data "opstracer_service" "checkout" {
  name = "checkout-api"
}

output "checkout_policy_id" {
  value = data.opstracer_service.checkout.escalation_policy_id
}
