data "opstracer_user" "alice" {
  email = "alice@example.com"
}

data "opstracer_user" "bob" {
  email = "bob@example.com"
}

# Weekly rotation, plus a business-hours layer where both are on call together.
resource "opstracer_schedule" "backend" {
  name      = "Backend on-call"
  time_zone = "Asia/Ho_Chi_Minh"

  # Needs the Slack integration connected.
  announcement = {
    slack_channel_id   = "C0123456789"
    slack_channel_name = "backend-oncall"
  }

  layers = [
    {
      name                  = "Weekly primary"
      rotation_start_at     = "2026-10-19T09:00:00+07:00"
      member_period_seconds = 604800
      participant_groups    = [[data.opstracer_user.alice.id], [data.opstracer_user.bob.id]]
    },
    {
      name                  = "Business hours"
      rotation_start_at     = "2026-10-19T09:00:00+07:00"
      member_period_seconds = 86400
      participant_groups    = [[data.opstracer_user.alice.id, data.opstracer_user.bob.id]]
      restrictions = [
        { days = [1, 2, 3, 4, 5], start_time_of_day = 32400, duration_seconds = 28800 },
      ]
    },
  ]
}
