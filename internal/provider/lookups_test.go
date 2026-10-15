package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestPlaybookFlow(t *testing.T) {
	srv := newFakeAPI(t)

	config := func(name, x string) string {
		return providerConfig(srv.URL) + `
resource "opstracer_playbook_flow" "db" {
  name        = "` + name + `"
  description = "Check the database first"
  graph = jsonencode({
    nodes = [
      { id = "incident", type = "incident", position = { x = 0, y = 0 } },
      { id = "db", type = "playbook", playbook = "database", position = { x = ` + x + `, y = 0.5 }, values = { env = "prod" } },
      { id = "end", type = "end", position = { x = 400, y = 0 } },
    ]
    edges = [
      { source = "incident", source_handle = "start", target = "db", target_handle = "after" },
      { source = "db", source_handle = "done", target = "end", target_handle = "after" },
    ]
  })
}

# Hand-written JSON with whitespace, a different key order and empty fields must not diff.
resource "opstracer_playbook_flow" "raw" {
  name       = "raw"
  visibility = "private"
  graph      = <<-JSON
    {
      "edges": [],
      "nodes": [
        {"type": "incident", "id": "incident", "playbook": null, "values": {}, "entities": [], "addons": {},
         "position": {"y": 0.0, "x": 0.0}}
      ]
    }
  JSON
}
`
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig(srv.URL) + `
resource "opstracer_playbook_flow" "bad" {
  name  = "bad"
  graph = "[1]"
}
`,
				ExpectError: regexp.MustCompile(`Invalid JSON object`),
			},
			{
				Config: config("db-first", "200"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("opstracer_playbook_flow.db", "id"),
					resource.TestCheckResourceAttr("opstracer_playbook_flow.db", "visibility", "org"),
					resource.TestCheckResourceAttr("opstracer_playbook_flow.db", "version", "1"),
					resource.TestCheckResourceAttr("opstracer_playbook_flow.raw", "visibility", "private"),
				),
			},
			{
				Config: config("db-first-v2", "250"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("opstracer_playbook_flow.db", "name", "db-first-v2"),
					resource.TestCheckResourceAttr("opstracer_playbook_flow.db", "version", "2"),
					resource.TestCheckResourceAttr("opstracer_playbook_flow.raw", "version", "1"),
				),
			},
			{ResourceName: "opstracer_playbook_flow.db", ImportState: true, ImportStateVerify: true},
			{
				// Import stores the server's canonical form, which matches jsonencode() but not hand-written JSON.
				ResourceName:            "opstracer_playbook_flow.raw",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"graph"},
			},
		},
	})
}

func TestLookupDataSources(t *testing.T) {
	srv := newFakeAPI(t)

	base := providerConfig(srv.URL) + `
data "opstracer_user" "me" {
  email = "` + fakeEmail + `"
}

resource "opstracer_team" "platform" {
  name            = "platform"
  member_user_ids = [data.opstracer_user.me.id]
}

resource "opstracer_schedule" "platform" {
  name      = "platform-oncall"
  time_zone = "Asia/Ho_Chi_Minh"
  layers = [{
    rotation_start_at     = "2026-10-19T09:00:00+07:00"
    member_period_seconds = 604800
    participant_groups    = [[data.opstracer_user.me.id]]
  }]
}

resource "opstracer_escalation_policy" "platform" {
  name        = "platform"
  description = "Platform paging"
  levels      = [{ targets = [{ type = "schedule", schedule_id = opstracer_schedule.platform.id }] }]
}

resource "opstracer_service" "api" {
  name                 = "platform-api"
  escalation_policy_id = opstracer_escalation_policy.platform.id
}
`
	lookups := base + `
data "opstracer_service" "api" {
  name = opstracer_service.api.name
}

data "opstracer_escalation_policy" "platform" {
  name = opstracer_escalation_policy.platform.name
}

data "opstracer_escalation_policy" "default" {
  default = true
}

data "opstracer_team" "platform" {
  name = opstracer_team.platform.name
}

data "opstracer_schedule" "platform" {
  name = opstracer_schedule.platform.name
}

data "opstracer_oncall" "during" {
  schedule_id = data.opstracer_schedule.platform.id
  at          = "2026-10-20T10:00:00Z"
}

data "opstracer_oncall" "before" {
  schedule_id = data.opstracer_schedule.platform.id
  at          = "2026-10-01T00:00:00Z"
}
`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: base + `
data "opstracer_escalation_policy" "both" {
  name    = "platform"
  default = true
}
`,
				ExpectError: regexp.MustCompile(`Invalid Attribute Combination`),
			},
			{
				Config: lookups,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.opstracer_service.api", "id", "opstracer_service.api", "id"),
					resource.TestCheckNoResourceAttr("data.opstracer_service.api", "description"),
					resource.TestCheckResourceAttrPair("data.opstracer_service.api", "escalation_policy_id", "opstracer_escalation_policy.platform", "id"),
					resource.TestCheckResourceAttrPair("data.opstracer_escalation_policy.platform", "id", "opstracer_escalation_policy.platform", "id"),
					resource.TestCheckResourceAttr("data.opstracer_escalation_policy.platform", "description", "Platform paging"),
					resource.TestCheckResourceAttr("data.opstracer_escalation_policy.platform", "is_default", "false"),
					resource.TestCheckResourceAttr("data.opstracer_escalation_policy.default", "id", fakeDefaultPolicyID),
					resource.TestCheckResourceAttr("data.opstracer_escalation_policy.default", "name", "Default policy"),
					resource.TestCheckResourceAttr("data.opstracer_escalation_policy.default", "is_default", "true"),
					resource.TestCheckResourceAttrPair("data.opstracer_team.platform", "id", "opstracer_team.platform", "id"),
					resource.TestCheckTypeSetElemAttr("data.opstracer_team.platform", "member_user_ids.*", fakeUserID),
					resource.TestCheckResourceAttrPair("data.opstracer_schedule.platform", "id", "opstracer_schedule.platform", "id"),
					resource.TestCheckResourceAttr("data.opstracer_schedule.platform", "time_zone", "Asia/Ho_Chi_Minh"),
					resource.TestCheckResourceAttr("data.opstracer_oncall.during", "user_ids.#", "1"),
					resource.TestCheckResourceAttr("data.opstracer_oncall.during", "user_ids.0", fakeUserID),
					resource.TestCheckResourceAttr("data.opstracer_oncall.during", "until", "2026-10-26T02:00:00Z"),
					resource.TestCheckResourceAttr("data.opstracer_oncall.before", "user_ids.#", "0"),
					resource.TestCheckResourceAttr("data.opstracer_oncall.before", "until", "2026-10-19T02:00:00Z"),
				),
			},
			{
				Config:      base + `data "opstracer_service" "missing" { name = "nope" }`,
				ExpectError: regexp.MustCompile(`no service is named "nope"`),
			},
			{
				Config: base + `
resource "opstracer_team" "dup" {
  count = 2
  name  = "dup"
}

data "opstracer_team" "dup" {
  name       = "dup"
  depends_on = [opstracer_team.dup]
}
`,
				ExpectError: regexp.MustCompile(`2 teams are named "dup"`),
			},
			{Config: lookups},
		},
	})
}
