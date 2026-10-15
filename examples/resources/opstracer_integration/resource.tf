resource "opstracer_integration" "checkout_datadog" {
  service_id        = opstracer_service.checkout.id
  name              = "checkout-datadog"
  type              = "datadog"
  default_severity  = "P3"
  rate_soft_per_min = 300
  severity_mapping = {
    critical = "P1"
    warning  = "P3"
  }

  # Change this value to issue a new routing key.
  rotate_key_trigger = "2026-10"
}

output "checkout_datadog_url" {
  value = opstracer_integration.checkout_datadog.integration_url
}

output "checkout_datadog_key" {
  value     = opstracer_integration.checkout_datadog.routing_key
  sensitive = true
}
