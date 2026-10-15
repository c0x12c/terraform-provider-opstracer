resource "opstracer_log_alert_monitor" "api" {
  source_type    = "railway"
  integration_id = "<railway-integration-id>"
  scope = {
    project_id     = "<railway-project-id>"
    environment_id = "<railway-environment-id>"
    service_id     = "<railway-service-id>"
  }
}
