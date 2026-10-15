data "opstracer_schedule" "backend" {
  name = "backend-oncall"
}

data "opstracer_oncall" "now" {
  schedule_id = data.opstracer_schedule.backend.id
}

output "on_call_user_ids" {
  value = data.opstracer_oncall.now.user_ids
}

output "shift_ends_at" {
  value = data.opstracer_oncall.now.until
}
