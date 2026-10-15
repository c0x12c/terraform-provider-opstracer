data "opstracer_escalation_policy" "backend" {
  name = "backend"
}

# The org's default policy, whatever it is called.
data "opstracer_escalation_policy" "default" {
  default = true
}
