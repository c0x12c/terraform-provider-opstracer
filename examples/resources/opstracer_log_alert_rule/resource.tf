resource "opstracer_log_alert_rule" "api_errors" {
  monitor_id     = opstracer_log_alert_monitor.api.id
  name           = "API error burst"
  match_level    = "error"
  threshold      = 10
  window_minutes = 5
  severity       = "P2"
}

resource "opstracer_log_alert_rule" "api_oom" {
  monitor_id     = opstracer_log_alert_monitor.api.id
  name           = "API out of memory"
  match_text     = ["OutOfMemoryError", "OOMKilled"]
  threshold      = 0
  window_minutes = 1
  severity       = "P1"
}
