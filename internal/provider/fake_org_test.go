package provider

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Fake routes for org invitations and members, schedule overrides and announcements, and the default
// escalation policy. Shapes follow what dev returns: null fields and empty arrays omitted, UTC timestamps.
func init() {
	fakeRoutes["POST /api/v1/org/invitations"] = fakeCreateInvitation
	fakeRoutes["POST /api/v1/org/invitations/list"] = fakeListInvitations
	fakeRoutes["POST /api/v1/org/invitations/resend"] = fakeResendInvitation
	fakeRoutes["POST /api/v1/org/invitations/revoke"] = fakeRevokeInvitation
	fakeRoutes["POST /fake/invitations/accept"] = fakeAcceptInvitation

	fakeRoutes["POST /api/v1/org/members/list"] = func(f *fakeAPI, w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		writePage(w, sortedValues(fakeMembers(f)))
	}
	fakeRoutes["POST /api/v1/org/members/update-role"] = fakeUpdateMember("role")
	fakeRoutes["POST /api/v1/org/members/update-title"] = fakeUpdateMember("title_id")

	fakeRoutes["POST /api/v1/schedule/overrides"] = fakeCreateOverride
	fakeRoutes["POST /api/v1/schedule/overrides/list"] = fakeListOverrides
	fakeRoutes["POST /api/v1/schedule/override/delete"] = func(f *fakeAPI, w http.ResponseWriter, _ *http.Request, body map[string]any) {
		id, _ := body["id"].(string)
		if _, ok := f.store("overrides")[id]; !ok {
			fakeError(w, http.StatusNotFound, "override_not_found")
			return
		}
		delete(f.store("overrides"), id)
		w.WriteHeader(http.StatusOK) // dev answers with an empty body
	}

	fakeRoutes["GET /api/v1/schedule/announcement"] = func(f *fakeAPI, w http.ResponseWriter, r *http.Request, _ map[string]any) {
		fakeAnnouncement(f, w, r.URL.Query().Get("schedule_id"), nil)
	}
	fakeRoutes["POST /api/v1/schedule/announcement/update"] = func(f *fakeAPI, w http.ResponseWriter, _ *http.Request, body map[string]any) {
		id, _ := body["schedule_id"].(string)
		fakeAnnouncement(f, w, id, body)
	}

	fakeRoutes["POST /api/v1/escalation-policies/set-default"] = func(f *fakeAPI, w http.ResponseWriter, _ *http.Request, body map[string]any) {
		id, _ := body["id"].(string)
		if _, ok := f.policies[id]; !ok {
			http.NotFound(w, nil)
			return
		}
		for pid, p := range f.policies {
			p["is_default"] = pid == id
		}
		writeJSON(w, f.policies[id])
	}
	// Updating a policy keeps its default flag.
	fakeRoutes["POST /api/v1/escalation-policies/update"] = func(f *fakeAPI, w http.ResponseWriter, _ *http.Request, body map[string]any) {
		id, _ := body["id"].(string)
		wasDefault := f.policies[id]["is_default"] == true
		f.update(w, f.policies, body, func(b map[string]any) map[string]any {
			b = policyView(b)
			b["is_default"] = wasDefault
			return b
		})
	}
}

func fakeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"error":{"code":%q,"message":%q}}`, code, code)
}

// Creating again for an email with a pending invitation updates it in place and issues a new link.
func fakeCreateInvitation(f *fakeAPI, w http.ResponseWriter, _ *http.Request, body map[string]any) {
	role, _ := body["role"].(string)
	if role != "ADMIN" && role != "MEMBER" {
		fakeError(w, http.StatusBadRequest, "invalid_role")
		return
	}
	email := strings.ToLower(body["email"].(string))
	var inv map[string]any
	for _, i := range f.store("invitations") {
		if i["email"] == email && i["status"] == "pending" {
			inv = i
			inv["resend_count"] = inv["resend_count"].(int) + 1
		}
	}
	if inv == nil {
		f.seq++
		inv = map[string]any{
			"id": fmt.Sprintf("00000000-0000-0000-0001-%012d", f.seq), "email": email, "status": "pending",
			"invited_by": fakeUserID, "resend_count": 0, "delivery_failed": false,
			"created_at": time.Now().UTC().Format(time.RFC3339Nano),
		}
		f.store("invitations")[inv["id"].(string)] = inv
	}
	inv["role"] = role
	inv["email_sent"] = body["send_email"] == true
	delete(inv, "teams")
	if ids, _ := body["team_ids"].([]any); len(ids) > 0 {
		teams := []any{}
		for _, id := range ids {
			teams = append(teams, map[string]any{"id": id, "name": "team", "is_default": false})
		}
		inv["teams"] = teams
	}
	writeInvitationWithURL(f, w, inv)
}

func writeInvitationWithURL(f *fakeAPI, w http.ResponseWriter, inv map[string]any) {
	f.seq++
	out := map[string]any{"invite_url": fmt.Sprintf("https://app.example.com/invite/token-%d", f.seq)}
	for k, v := range inv {
		out[k] = v
	}
	writeJSON(w, out)
}

func fakeListInvitations(f *fakeAPI, w http.ResponseWriter, _ *http.Request, body map[string]any) {
	status, _ := body["status"].(string)
	out := []any{}
	for _, inv := range sortedValues(f.store("invitations")) {
		if status == "" || inv.(map[string]any)["status"] == status {
			out = append(out, inv)
		}
	}
	writePage(w, out)
}

func pendingInvitation(f *fakeAPI, w http.ResponseWriter, r *http.Request) map[string]any {
	inv, ok := f.store("invitations")[r.URL.Query().Get("id")]
	if !ok || inv["status"] != "pending" {
		fakeError(w, http.StatusGone, "invitation_gone")
		return nil
	}
	return inv
}

func fakeResendInvitation(f *fakeAPI, w http.ResponseWriter, r *http.Request, body map[string]any) {
	if inv := pendingInvitation(f, w, r); inv != nil {
		inv["resend_count"] = inv["resend_count"].(int) + 1
		inv["email_sent"] = body["send_email"] == true
		writeInvitationWithURL(f, w, inv)
	}
}

func fakeRevokeInvitation(f *fakeAPI, w http.ResponseWriter, r *http.Request, _ map[string]any) {
	if inv := pendingInvitation(f, w, r); inv != nil {
		inv["status"] = "revoked"
		w.WriteHeader(http.StatusOK)
	}
}

// fakeAcceptInvitation is a test hook: the invitee joins, and the invitation leaves the list.
func fakeAcceptInvitation(f *fakeAPI, w http.ResponseWriter, r *http.Request, _ map[string]any) {
	if inv := pendingInvitation(f, w, r); inv != nil {
		delete(f.store("invitations"), inv["id"].(string))
		f.seq++
		id := fmt.Sprintf("00000000-0000-0000-0002-%012d", f.seq)
		fakeMembers(f)[id] = map[string]any{"user_id": id, "email": inv["email"], "role": inv["role"], "banned": false}
		w.WriteHeader(http.StatusOK)
	}
}

func fakeMembers(f *fakeAPI) map[string]map[string]any {
	members := f.store("members")
	if len(members) == 0 {
		members[fakeUserID] = map[string]any{"user_id": fakeUserID, "email": fakeEmail, "display_name": "On Call", "role": "OWNER", "banned": false}
	}
	return members
}

func fakeUpdateMember(field string) fakeRoute {
	return func(f *fakeAPI, w http.ResponseWriter, r *http.Request, body map[string]any) {
		m, ok := fakeMembers(f)[r.URL.Query().Get("user_id")]
		if !ok {
			fakeError(w, http.StatusNotFound, "member_not_found")
			return
		}
		switch field {
		case "role":
			m["role"] = body["role"]
		case "title_id":
			delete(m, "title")
			if id, _ := body["title_id"].(string); id != "" {
				t, ok := f.titles[id]
				if !ok {
					fakeError(w, http.StatusNotFound, "title_not_found")
					return
				}
				m["title"] = map[string]any{"id": id, "name": t["name"]}
			}
		}
		writeJSON(w, m)
	}
}

func fakeTime(v any) (time.Time, bool) {
	s, _ := v.(string)
	t, err := time.Parse(time.RFC3339, s)
	return t, err == nil
}

func fakeCreateOverride(f *fakeAPI, w http.ResponseWriter, _ *http.Request, body map[string]any) {
	if _, ok := f.schedules[body["schedule_id"].(string)]; !ok {
		fakeError(w, http.StatusNotFound, "schedule_not_found")
		return
	}
	start, okStart := fakeTime(body["starts_at"])
	end, okEnd := fakeTime(body["ends_at"])
	if !okStart || !okEnd || !end.After(start) {
		fakeError(w, http.StatusBadRequest, "invalid_override_window")
		return
	}
	f.seq++
	o := map[string]any{
		"id": fmt.Sprintf("00000000-0000-0000-0003-%012d", f.seq), "schedule_id": body["schedule_id"],
		"user_id": body["user_id"], "starts_at": toUTC(body["starts_at"]), "ends_at": toUTC(body["ends_at"]),
		"created_by": fakeUserID, "created_at": time.Now().UTC().Format(time.RFC3339Nano),
	}
	if v, ok := body["replaces_user_id"].(string); ok {
		o["replaces_user_id"] = v
	}
	f.store("overrides")[o["id"].(string)] = o
	writeJSON(w, o)
}

// fakeListOverrides returns overrides overlapping [from, to) as a plain, unpaginated array.
func fakeListOverrides(f *fakeAPI, w http.ResponseWriter, _ *http.Request, body map[string]any) {
	from, okFrom := fakeTime(body["from"])
	to, okTo := fakeTime(body["to"])
	switch {
	case !okFrom || !okTo || !to.After(from):
		fakeError(w, http.StatusBadRequest, "invalid_range")
		return
	case to.Sub(from) > 90*24*time.Hour:
		fakeError(w, http.StatusBadRequest, "range_too_wide")
		return
	}
	if _, ok := f.schedules[body["schedule_id"].(string)]; !ok {
		fakeError(w, http.StatusNotFound, "schedule_not_found")
		return
	}
	out := []map[string]any{}
	for _, o := range f.store("overrides") {
		start, _ := fakeTime(o["starts_at"])
		end, _ := fakeTime(o["ends_at"])
		if o["schedule_id"] == body["schedule_id"] && start.Before(to) && end.After(from) {
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["starts_at"].(string) < out[j]["starts_at"].(string) })
	writeJSON(w, out)
}

// fakeAnnouncement reads, or with body updates, a schedule's announcement as if Slack were connected.
func fakeAnnouncement(f *fakeAPI, w http.ResponseWriter, scheduleID string, body map[string]any) {
	if _, ok := f.schedules[scheduleID]; !ok {
		fakeError(w, http.StatusNotFound, "schedule_not_found")
		return
	}
	a := f.store("announcements")[scheduleID]
	if a == nil || body != nil {
		a = map[string]any{"schedule_id": scheduleID, "enabled": false}
		for _, k := range []string{"slack_channel_id", "slack_channel_name"} {
			if v, ok := body[k].(string); ok {
				a[k] = v
				a["enabled"] = true
			}
		}
		f.store("announcements")[scheduleID] = a
	}
	writeJSON(w, a)
}
