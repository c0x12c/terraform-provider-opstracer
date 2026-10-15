# Evaluated top to bottom; the first match picks the policy.
resource "opstracer_routing_rules" "org" {
  rules = [
    {
      escalation_policy_id = opstracer_escalation_policy.backend.id
      severities           = ["P1", "P2"]
      source_type          = "datadog"
      tags                 = { team = "backend", env = "prod" }
    },
    {
      escalation_policy_id = opstracer_escalation_policy.backend.id
      enabled              = false
      tags                 = { env = "staging" }
    },
  ]
}
