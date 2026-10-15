data "opstracer_user" "lead" {
  email = "lead@example.com"
}

resource "opstracer_escalation_policy" "backend" {
  name        = "Backend"
  description = "Page the backend on-call, then the lead"
  is_default  = true

  levels = [
    {
      timeout_seconds = 600
      targets         = [{ type = "schedule", schedule_id = opstracer_schedule.backend.id }]
    },
    {
      targets = [{ type = "user", user_id = data.opstracer_user.lead.id }]
    },
  ]
}
