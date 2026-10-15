package provider

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestTeamSettings(t *testing.T) {
	srv := newFakeAPI(t)

	config := func(teamName, days, ruleOrder, emailDelay string) string {
		return providerConfig(srv.URL) + `
data "opstracer_user" "me" {
  email = "` + strings.ToUpper(fakeEmail) + `"
}

resource "opstracer_member_title" "sre" {
  name = "SRE"
}

resource "opstracer_team" "backend" {
  name            = "` + teamName + `"
  member_user_ids = [data.opstracer_user.me.id]
}

resource "opstracer_schedule" "backend" {
  name = "backend-oncall"
  layers = [
    {
      name                  = "Weekly"
      rotation_start_at     = "2026-10-19T09:00:00+07:00"
      member_period_seconds = 604800
      participant_groups    = [[data.opstracer_user.me.id]]
    },
    {
      rotation_start_at     = "2026-10-19T09:00:00+07:00"
      member_period_seconds = 86400
      participant_groups    = [[data.opstracer_user.me.id]]
      restrictions          = [{ days = ` + days + `, start_time_of_day = 32400, duration_seconds = 28800 }]
    },
  ]
}

resource "opstracer_escalation_policy" "backend" {
  name   = "backend"
  levels = [{ targets = [{ type = "schedule", schedule_id = opstracer_schedule.backend.id }] }]
}

locals {
  rules = {
    critical = {
      escalation_policy_id = opstracer_escalation_policy.backend.id
      enabled              = true
      severities           = ["P1", "P2"]
      source_type          = "datadog"
      tags                 = { env = "prod" }
    }
    staging = {
      escalation_policy_id = opstracer_escalation_policy.backend.id
      enabled              = false
      severities           = null
      source_type          = null
      tags                 = { env = "staging" }
    }
  }
}

resource "opstracer_routing_rules" "org" {
  rules = [` + ruleOrder + `]
}

resource "opstracer_notification_order" "org" {
  enabled = true
  tiers = [
    { channels = ["slack", "telegram"] },
    { channels = ["email"], delay_seconds = ` + emailDelay + ` },
  ]
}
`
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config("backend", "[1, 2, 3, 4, 5]", "local.rules.critical, local.rules.staging", "120"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.opstracer_user.me", "id", fakeUserID),
					resource.TestCheckResourceAttr("opstracer_team.backend", "member_user_ids.#", "1"),
					resource.TestCheckResourceAttr("opstracer_schedule.backend", "time_zone", "Asia/Saigon"),
					resource.TestCheckResourceAttr("opstracer_schedule.backend", "layers.0.rotation_start_at", "2026-10-19T09:00:00+07:00"),
					resource.TestCheckResourceAttr("opstracer_schedule.backend", "layers.1.time_zone", "Asia/Saigon"),
					resource.TestCheckResourceAttr("opstracer_schedule.backend", "layers.1.restrictions.0.days.#", "5"),
					resource.TestCheckResourceAttr("opstracer_routing_rules.org", "rules.0.source_type", "datadog"),
					resource.TestCheckResourceAttr("opstracer_routing_rules.org", "rules.1.enabled", "false"),
					resource.TestCheckResourceAttr("opstracer_notification_order.org", "tiers.0.delay_seconds", "0"),
				),
			},
			{
				Config: config("backend-renamed", "[6, 7]", "local.rules.staging, local.rules.critical", "300"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("opstracer_team.backend", "name", "backend-renamed"),
					resource.TestCheckResourceAttr("opstracer_schedule.backend", "layers.1.restrictions.0.days.#", "2"),
					resource.TestCheckResourceAttr("opstracer_routing_rules.org", "rules.0.tags.env", "staging"),
					resource.TestCheckNoResourceAttr("opstracer_routing_rules.org", "rules.0.source_type"),
					resource.TestCheckResourceAttr("opstracer_routing_rules.org", "rules.1.source_type", "datadog"),
					resource.TestCheckResourceAttr("opstracer_notification_order.org", "tiers.1.delay_seconds", "300"),
				),
			},
			{ResourceName: "opstracer_team.backend", ImportState: true, ImportStateVerify: true},
			{ResourceName: "opstracer_member_title.sre", ImportState: true, ImportStateVerify: true},
			{ResourceName: "opstracer_routing_rules.org", ImportState: true, ImportStateId: "routing_rules", ImportStateVerify: true},
			{ResourceName: "opstracer_notification_order.org", ImportState: true, ImportStateId: "notification_order", ImportStateVerify: true},
			{
				// rotation_start_at comes back in UTC on import; that is the only expected difference.
				ResourceName:            "opstracer_schedule.backend",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"layers.0.rotation_start_at", "layers.1.rotation_start_at"},
			},
		},
	})
}
