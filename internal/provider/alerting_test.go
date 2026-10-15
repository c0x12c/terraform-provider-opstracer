package provider

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/c0x12c/terraform-provider-opstracer/internal/client"
)

func TestMuteAndSuppression(t *testing.T) {
	srv := newFakeAPI(t)
	c := client.New(srv.URL, fakeToken, "test")
	inAnHour := time.Now().Add(time.Hour).In(time.FixedZone("ICT", 7*3600)).Format(time.RFC3339)

	config := func(scheduledEnd string) string {
		return providerConfig(srv.URL) + `
resource "opstracer_escalation_policy" "p" {
  name   = "p"
  levels = [{ targets = [{ type = "user", user_id = "` + fakeUserID + `" }] }]
}

resource "opstracer_service" "api" {
  name                 = "api"
  escalation_policy_id = opstracer_escalation_policy.p.id
}

resource "opstracer_mute" "now" {
  scope_type = "service"
  scope_id   = opstracer_service.api.id
  ends_at    = "` + inAnHour + `"
  reason     = "deploy"
}

resource "opstracer_mute" "scheduled" {
  scope_type = "service"
  scope_id   = opstracer_service.api.id
  starts_at  = "2030-01-01T09:00:00+07:00"
  ends_at    = "` + scheduledEnd + `"
}

resource "opstracer_suppression" "noisy" {
  service_id  = opstracer_service.api.id
  severities  = ["P5", "P4"]
  source_type = "datadog"
  tags        = { env = "staging" }
  ends_at     = "` + inAnHour + `"
}
`
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(),
		CheckDestroy: func(*terraform.State) error {
			mutes, err := c.ListMutes(context.Background())
			if err != nil || len(mutes) > 0 {
				return fmt.Errorf("mutes left: %v %v", mutes, err)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: config("2030-01-01T10:00:00+07:00"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("opstracer_mute.now", "active", "true"),
					resource.TestCheckResourceAttr("opstracer_mute.now", "ends_at", inAnHour),
					resource.TestCheckResourceAttrSet("opstracer_mute.now", "starts_at"),
					resource.TestCheckResourceAttrPair("opstracer_mute.now", "service_id", "opstracer_service.api", "id"),
					resource.TestCheckResourceAttr("opstracer_mute.scheduled", "active", "false"),
					resource.TestCheckResourceAttr("opstracer_mute.scheduled", "starts_at", "2030-01-01T09:00:00+07:00"),
					resource.TestCheckNoResourceAttr("opstracer_mute.scheduled", "reason"),
					resource.TestCheckResourceAttr("opstracer_suppression.noisy", "severities.#", "2"),
					resource.TestCheckResourceAttr("opstracer_suppression.noisy", "active", "true"),
					resource.TestCheckResourceAttr("opstracer_suppression.noisy", "match_count", "0"),
				),
			},
			{
				// Times come back in UTC on import; that is the only expected difference.
				ResourceName:            "opstracer_mute.now",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"ends_at"},
			},
			{
				// Same instant spelled in UTC, as after an import: updated in place, not replaced.
				Config: config("2030-01-01T03:00:00Z"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("opstracer_mute.scheduled", plancheck.ResourceActionUpdate),
				}},
				Check: resource.TestCheckResourceAttr("opstracer_mute.scheduled", "ends_at", "2030-01-01T03:00:00Z"),
			},
			{
				ResourceName:            "opstracer_suppression.noisy",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"ends_at"},
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					r := s.RootModule().Resources["opstracer_suppression.noisy"].Primary
					return r.Attributes["service_id"] + "/" + r.ID, nil
				},
			},
			{
				Config: config("2030-01-01T11:00:00+07:00"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("opstracer_mute.scheduled", plancheck.ResourceActionDestroyBeforeCreate),
					plancheck.ExpectResourceAction("opstracer_mute.now", plancheck.ResourceActionNoop),
				}},
				Check: resource.TestCheckResourceAttr("opstracer_mute.scheduled", "ends_at", "2030-01-01T11:00:00+07:00"),
			},
		},
	})
}

// Expired windows drop out of the list but stay in state, inactive and without a diff.
func TestMuteAndSuppressionExpire(t *testing.T) {
	srv := newFakeAPI(t)
	end := time.Now().Add(5 * time.Second).Truncate(time.Second)
	config := providerConfig(srv.URL) + `
resource "opstracer_escalation_policy" "p" {
  name   = "p"
  levels = [{ targets = [{ type = "user", user_id = "` + fakeUserID + `" }] }]
}

resource "opstracer_service" "api" {
  name                 = "api"
  escalation_policy_id = opstracer_escalation_policy.p.id
}

resource "opstracer_mute" "short" {
  scope_type = "service"
  scope_id   = opstracer_service.api.id
  ends_at    = "` + end.Format(time.RFC3339) + `"
}

resource "opstracer_suppression" "short" {
  service_id = opstracer_service.api.id
  ends_at    = "` + end.Format(time.RFC3339) + `"
}
`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  resource.TestCheckResourceAttr("opstracer_mute.short", "active", "true"),
			},
			{
				PreConfig: func() { time.Sleep(time.Until(end) + time.Second) },
				Config:    config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("opstracer_mute.short", "active", "false"),
					resource.TestCheckResourceAttr("opstracer_suppression.short", "active", "false"),
				),
			},
		},
	})
}

func TestLogAlerts(t *testing.T) {
	srv := newFakeAPI(t)

	config := func(enabled, threshold string) string {
		return providerConfig(srv.URL) + `
resource "opstracer_log_alert_monitor" "api" {
  source_type    = "railway"
  integration_id = "33333333-3333-3333-3333-333333333333"
  scope          = { project_id = "p1", environment_id = "e1", service_id = "s1" }
  enabled        = ` + enabled + `
}

resource "opstracer_log_alert_rule" "errors" {
  monitor_id     = opstracer_log_alert_monitor.api.id
  name           = "API errors"
  match_level    = "error"
  threshold      = ` + threshold + `
  window_minutes = 5
  severity       = "P2"
}

resource "opstracer_log_alert_rule" "oom" {
  monitor_id     = opstracer_log_alert_monitor.api.id
  name           = "OOM"
  match_text     = ["OutOfMemory", "OOMKilled"]
  threshold      = 0
  window_minutes = 1
  severity       = "P1"
  enabled        = false
}
`
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config("true", "10"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("opstracer_log_alert_monitor.api", "enabled", "true"),
					resource.TestCheckResourceAttr("opstracer_log_alert_monitor.api", "scope_name", "api / production"),
					resource.TestCheckNoResourceAttr("opstracer_log_alert_rule.errors", "match_text"),
					resource.TestCheckResourceAttr("opstracer_log_alert_rule.errors", "enabled", "true"),
					resource.TestCheckResourceAttr("opstracer_log_alert_rule.errors", "state", "ok"),
					resource.TestCheckResourceAttr("opstracer_log_alert_rule.oom", "match_text.1", "OOMKilled"),
				),
			},
			{
				Config: config("false", "20"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("opstracer_log_alert_monitor.api", plancheck.ResourceActionUpdate),
				}},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("opstracer_log_alert_monitor.api", "enabled", "false"),
					resource.TestCheckResourceAttr("opstracer_log_alert_rule.errors", "threshold", "20"),
				),
			},
			{ResourceName: "opstracer_log_alert_monitor.api", ImportState: true, ImportStateVerify: true},
			{ResourceName: "opstracer_log_alert_rule.errors", ImportState: true, ImportStateVerify: true},
			{ResourceName: "opstracer_log_alert_rule.oom", ImportState: true, ImportStateVerify: true},
		},
	})
}
