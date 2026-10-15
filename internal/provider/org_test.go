package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// fakeCall sends a request to the fake API, for steps that simulate changes made outside Terraform.
func fakeCall(t *testing.T, method, url string) []byte {
	t.Helper()
	req, _ := http.NewRequest(method, url, nil)
	req.Header.Set("Authorization", "Bearer "+fakeToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		t.Fatalf("%s %s: HTTP %d: %s", method, url, resp.StatusCode, body)
	}
	return body
}

func attr(s *terraform.State, name, key string) string {
	return s.RootModule().Resources[name].Primary.Attributes[key]
}

func TestOrgInvitation(t *testing.T) {
	srv := newFakeAPI(t)
	const name = "opstracer_org_invitation.alice"

	config := func(role, teams, trigger string) string {
		return providerConfig(srv.URL) + `
resource "opstracer_team" "backend" {
  name = "backend"
}

resource "opstracer_org_invitation" "alice" {
  email          = "Alice@Example.com"
  role           = "` + role + `"
  team_ids       = ` + teams + `
  send_email     = false
  resend_trigger = "` + trigger + `"
}
`
	}

	var id, url string
	keep := func(s *terraform.State) error {
		id, url = attr(s, name, "id"), attr(s, name, "invite_url")
		return nil
	}
	sameIDNewURL := func(s *terraform.State) error {
		if attr(s, name, "id") != id || attr(s, name, "invite_url") == url {
			return fmt.Errorf("want same id and a new invite_url, got id %s", attr(s, name, "id"))
		}
		return keep(s)
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config("MEMBER", "null", "1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "email", "Alice@Example.com"),
					resource.TestCheckResourceAttr(name, "status", "pending"),
					resource.TestCheckNoResourceAttr(name, "team_ids"),
					resource.TestCheckResourceAttrSet(name, "invite_url"),
					keep,
				),
			},
			{
				Config: config("ADMIN", "[opstracer_team.backend.id]", "1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "role", "ADMIN"),
					resource.TestCheckResourceAttr(name, "team_ids.#", "1"),
					sameIDNewURL,
				),
			},
			{
				Config: config("ADMIN", "[opstracer_team.backend.id]", "2"),
				Check:  sameIDNewURL,
			},
			{
				ResourceName:      name,
				ImportState:       true,
				ImportStateVerify: true,
				// The server lowercases email; the config spelling is kept only when known.
				ImportStateVerifyIgnore: []string{"email", "invite_url", "send_email", "resend_trigger"},
			},
			{
				// Revoked outside Terraform: planned for re-creation.
				PreConfig: func() { fakeCall(t, http.MethodPost, srv.URL+"/api/v1/org/invitations/revoke?id="+id) },
				Config:    config("ADMIN", "[opstracer_team.backend.id]", "2"),
				Check: func(s *terraform.State) error {
					if attr(s, name, "id") == id {
						return fmt.Errorf("revoked invitation was not re-created")
					}
					return keep(s)
				},
			},
			{
				// Accepted: it leaves the list but stays in state, with no diff.
				PreConfig: func() { fakeCall(t, http.MethodPost, srv.URL+"/fake/invitations/accept?id="+id) },
				Config:    config("ADMIN", "[opstracer_team.backend.id]", "2"),
				Check:     resource.TestCheckResourceAttr(name, "status", "accepted"),
			},
		},
	})
}

func TestOrgMember(t *testing.T) {
	srv := newFakeAPI(t)
	const name = "opstracer_org_member.me"

	config := func(role, title string) string {
		return providerConfig(srv.URL) + `
resource "opstracer_member_title" "sre" {
  name = "SRE"
}

resource "opstracer_org_member" "me" {
  user_id  = "` + fakeUserID + `"
  role     = ` + role + `
  title_id = ` + title + `
}
`
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(),
		CheckDestroy: func(*terraform.State) error {
			var page struct {
				Data struct {
					Data []map[string]any `json:"data"`
				} `json:"data"`
			}
			_ = json.Unmarshal(fakeCall(t, http.MethodPost, srv.URL+"/api/v1/org/members/list"), &page)
			if len(page.Data.Data) != 1 {
				return fmt.Errorf("destroy must not remove the member")
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: config("null", "opstracer_member_title.sre.id"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "id", fakeUserID),
					resource.TestCheckResourceAttr(name, "role", "OWNER"),
					resource.TestCheckResourceAttr(name, "email", fakeEmail),
					resource.TestCheckResourceAttrPair(name, "title_id", "opstracer_member_title.sre", "id"),
				),
			},
			{
				Config: config(`"ADMIN"`, "null"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "role", "ADMIN"),
					resource.TestCheckNoResourceAttr(name, "title_id"),
				),
			},
			{
				ResourceName:      name,
				ImportState:       true,
				ImportStateId:     fakeUserID,
				ImportStateVerify: true,
			},
			{
				Config:      providerConfig(srv.URL) + `resource "opstracer_org_member" "ghost" { user_id = "00000000-0000-0000-0000-00000000dead" }`,
				ExpectError: regexp.MustCompile("not a member of the org"),
			},
		},
	})
}

func TestScheduleOverrideAnnouncementAndDefaultPolicy(t *testing.T) {
	srv := newFakeAPI(t)
	const override = "opstracer_schedule_override.cover"
	zone := time.FixedZone("ICT", 7*3600)
	start := time.Now().In(zone).Add(30 * 24 * time.Hour).Truncate(time.Hour)
	ts := func(t time.Time) string { return t.Format(time.RFC3339) }

	config := func(end time.Time, announcement, defaultA, defaultB string) string {
		return providerConfig(srv.URL) + `
resource "opstracer_schedule" "s" {
  name = "backend"
  layers = [{
    rotation_start_at     = "2026-10-19T09:00:00+07:00"
    member_period_seconds = 604800
    participant_groups    = [["` + fakeUserID + `"]]
  }]
  announcement = ` + announcement + `
}

resource "opstracer_schedule_override" "cover" {
  schedule_id      = opstracer_schedule.s.id
  user_id          = "` + fakeUserID + `"
  starts_at        = "` + ts(start) + `"
  ends_at          = "` + ts(end) + `"
  replaces_user_id = "` + fakeUserID + `"
}

resource "opstracer_escalation_policy" "a" {
  name       = "a"
  levels     = [{ targets = [{ type = "user", user_id = "` + fakeUserID + `" }] }]
  is_default = ` + defaultA + `
}

resource "opstracer_escalation_policy" "b" {
  name       = "b"
  levels     = [{ targets = [{ type = "user", user_id = "` + fakeUserID + `" }] }]
  is_default = ` + defaultB + `
}
`
	}

	var overrideID string
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config(start.Add(24*time.Hour), `{ slack_channel_id = "C0123", slack_channel_name = "oncall" }`, "true", "null"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(override, "starts_at", ts(start)),
					resource.TestCheckResourceAttr(override, "replaces_user_id", fakeUserID),
					resource.TestCheckResourceAttr("opstracer_schedule.s", "announcement.slack_channel_id", "C0123"),
					resource.TestCheckResourceAttr("opstracer_escalation_policy.a", "is_default", "true"),
					resource.TestCheckResourceAttr("opstracer_escalation_policy.b", "is_default", "false"),
					func(s *terraform.State) error { overrideID = attr(s, override, "id"); return nil },
				),
			},
			{
				// Moving the default: true on the new policy, unset on the old one.
				Config: config(start.Add(48*time.Hour), "null", "null", "true"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("opstracer_schedule.s", "announcement"),
					resource.TestCheckResourceAttr("opstracer_escalation_policy.b", "is_default", "true"),
					func(s *terraform.State) error {
						if attr(s, override, "id") == overrideID {
							return fmt.Errorf("changing ends_at must replace the override")
						}
						return nil
					},
				),
			},
			{
				// a only learns it lost the default on the next refresh.
				Config: config(start.Add(48*time.Hour), "null", "null", "true"),
				Check:  resource.TestCheckResourceAttr("opstracer_escalation_policy.a", "is_default", "false"),
			},
			{
				ResourceName: override,
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return attr(s, "opstracer_schedule.s", "id") + "/" + attr(s, override, "id"), nil
				},
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"starts_at", "ends_at"},
			},
			{ResourceName: "opstracer_schedule.s", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"layers.0.rotation_start_at"}},
			{
				Config:      config(start.Add(48*time.Hour), "null", "null", "false"),
				ExpectError: regexp.MustCompile(`Cannot unset the default`),
			},
		},
	})
}
