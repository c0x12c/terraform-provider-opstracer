resource "opstracer_service_data_sources" "checkout" {
  service_id = opstracer_service.checkout.id
  sources = {
    grafana = { choice = "connection", connection_id = opstracer_observability_connection.grafana.id }
    sentry  = { choice = "off" }
  }
}
