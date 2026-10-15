# Silence only low-severity Datadog alerts from staging, for one day.
resource "opstracer_suppression" "staging_noise" {
  service_id  = opstracer_service.checkout.id
  severities  = ["P4", "P5"]
  source_type = "datadog"
  tags        = { env = "staging" }
  ends_at     = "2026-11-02T09:00:00+07:00"
  reason      = "Noisy staging monitors"
}
