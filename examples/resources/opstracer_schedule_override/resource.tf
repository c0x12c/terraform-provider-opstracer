data "opstracer_user" "bob" {
  email = "bob@example.com"
}

# Bob covers the backend rotation for one day.
resource "opstracer_schedule_override" "bob_covers" {
  schedule_id = opstracer_schedule.backend.id
  user_id     = data.opstracer_user.bob.id
  starts_at   = "2026-11-20T09:00:00+07:00"
  ends_at     = "2026-11-21T09:00:00+07:00"
}
