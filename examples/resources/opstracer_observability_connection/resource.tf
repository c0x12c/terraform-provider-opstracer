variable "grafana_token" {
  type      = string
  sensitive = true
}

resource "opstracer_observability_connection" "grafana" {
  type  = "grafana"
  name  = "grafana-prod"
  site  = "https://acme.grafana.net"
  token = var.grafana_token
}
