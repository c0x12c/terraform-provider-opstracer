terraform {
  required_providers {
    opstracer = {
      source = "c0x12c/opstracer"
    }
  }
}

provider "opstracer" {}

data "opstracer_user" "alice" {
  email = "alice@example.com"
}

data "opstracer_user" "bob" {
  email = "bob@example.com"
}

resource "opstracer_team" "backend" {
  name            = "Backend"
  member_user_ids = [data.opstracer_user.alice.id, data.opstracer_user.bob.id]
}

resource "opstracer_member_title" "sre" {
  name = "SRE"
}

# Weekly rotation between Alice and Bob, plus a business-hours layer where both are on together.
resource "opstracer_schedule" "backend" {
  name      = "Backend on-call"
  time_zone = "Asia/Ho_Chi_Minh"

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

resource "opstracer_escalation_policy" "backend" {
  name = "Backend"
  levels = [
    { timeout_seconds = 300, targets = [{ type = "schedule", schedule_id = opstracer_schedule.backend.id }] },
    { targets = [{ type = "user", user_id = data.opstracer_user.alice.id }] },
  ]
}

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

resource "opstracer_notification_order" "org" {
  enabled = true
  tiers = [
    { channels = ["slack", "push"] },
    { channels = ["sms"], delay_seconds = 120 },
    { channels = ["voice"], delay_seconds = 300 },
  ]
}
