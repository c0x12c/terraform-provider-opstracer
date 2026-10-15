# Silence every alert of the checkout service during a maintenance window.
resource "opstracer_mute" "checkout_maintenance" {
  scope_type = "service"
  scope_id   = opstracer_service.checkout.id
  starts_at  = "2026-11-01T22:00:00+07:00"
  ends_at    = "2026-11-02T01:00:00+07:00"
  reason     = "Database upgrade"
}
