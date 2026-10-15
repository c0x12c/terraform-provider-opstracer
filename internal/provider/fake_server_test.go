package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

const (
	fakeToken  = "test-token"
	fakeUserID = "08be2646-0000-0000-0000-000000000001"
	fakeEmail  = "oncall@example.com"
)

// fakeAPI mimics the control-plane behaviour seen on dev: snake_case bodies wrapped in {"data": ...},
// 404 for missing objects, server-side defaults, empty arrays omitted, timestamps returned in UTC.
type fakeAPI struct {
	mu                sync.Mutex
	seq               int
	services          map[string]map[string]any
	policies          map[string]map[string]any
	schedules         map[string]map[string]any
	teams             map[string]map[string]any
	titles            map[string]map[string]any
	rules             []map[string]any
	notificationOrder map[string]any
	stores            map[string]map[string]map[string]any
}

// fakeRoute serves one "METHOD /path" of the fake API. Each API area registers its routes from its
// own fake_<area>_test.go init(), so areas can be added without touching this file.
type fakeRoute func(f *fakeAPI, w http.ResponseWriter, r *http.Request, body map[string]any)

var fakeRoutes = map[string]fakeRoute{}

// store returns the named object store, creating it on first use.
func (f *fakeAPI) store(name string) map[string]map[string]any {
	if f.stores[name] == nil {
		f.stores[name] = map[string]map[string]any{}
	}
	return f.stores[name]
}

func newFakeAPI(t *testing.T) *httptest.Server {
	f := &fakeAPI{
		stores:    map[string]map[string]map[string]any{},
		services:  map[string]map[string]any{},
		policies:  map[string]map[string]any{},
		schedules: map[string]map[string]any{},
		teams:     map[string]map[string]any{},
		titles: map[string]map[string]any{
			"system-engineer": {"id": "system-engineer", "name": "Engineer", "scope": "SYSTEM", "member_count": 0},
		},
		notificationOrder: map[string]any{"schema_version": 1, "enabled": false,
			"tiers": []any{map[string]any{"channels": []any{"telegram"}, "delay_seconds": 0}}},
	}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return srv
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+fakeToken {
		http.Error(w, `{"code":"token_invalid"}`, http.StatusUnauthorized)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	var body map[string]any
	if r.Method == http.MethodPost {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	id := r.URL.Query().Get("id")

	key := r.Method + " " + r.URL.Path
	if h, ok := fakeRoutes[key]; ok {
		h(f, w, r, body)
		return
	}
	switch key {
	case "POST /api/v1/escalation-policies":
		f.create(w, f.policies, body, policyView)
	case "GET /api/v1/escalation-policy":
		f.get(w, f.policies, id)
	case "POST /api/v1/escalation-policies/update":
		f.update(w, f.policies, body, policyView)
	case "POST /api/v1/escalation-policies/delete":
		f.delete(w, f.policies, body)

	case "POST /api/v1/services":
		f.create(w, f.services, body, f.serviceView)
	case "GET /api/v1/service":
		f.get(w, f.services, id)
	case "POST /api/v1/services/update":
		f.update(w, f.services, body, f.serviceView)
	case "POST /api/v1/services/delete":
		f.delete(w, f.services, body)

	case "POST /api/v1/schedules":
		f.create(w, f.schedules, body, scheduleView)
	case "GET /api/v1/schedule":
		f.get(w, f.schedules, id)
	case "POST /api/v1/schedule/update":
		f.update(w, f.schedules, body, scheduleView)
	case "POST /api/v1/schedule/delete":
		f.delete(w, f.schedules, body)

	case "POST /api/v1/org/teams":
		body["members"] = []any{}
		f.create(w, f.teams, body, identity)
	case "GET /api/v1/org/team":
		f.get(w, f.teams, id)
	case "POST /api/v1/org/teams/update":
		if t, ok := f.teams[body["id"].(string)]; ok {
			t["name"] = body["name"]
		}
		f.get(w, f.teams, body["id"].(string))
	case "POST /api/v1/org/teams/delete":
		f.delete(w, f.teams, body)
	case "POST /api/v1/org/teams/members/add":
		f.addTeamMembers(w, body)
	case "POST /api/v1/org/teams/members/remove":
		f.removeTeamMember(w, body)

	case "GET /api/v1/org/titles":
		writeJSON(w, sortedValues(f.titles))
	case "POST /api/v1/org/titles":
		body["scope"] = "ORG"
		f.create(w, f.titles, body, identity)
	case "POST /api/v1/org/titles/update":
		f.update(w, f.titles, body, func(b map[string]any) map[string]any { b["scope"] = "ORG"; return b })
	case "POST /api/v1/org/titles/delete":
		f.delete(w, f.titles, body)

	case "POST /api/v1/routing-rules/list":
		writePage(w, f.ruleViews())
	case "POST /api/v1/routing-rules":
		f.seq++
		body["id"] = fmt.Sprintf("rule-%d", f.seq)
		f.rules = append(f.rules, body)
		writeJSON(w, f.ruleView(len(f.rules)-1))
	case "POST /api/v1/routing-rules/update":
		f.updateRule(w, body)
	case "POST /api/v1/routing-rules/delete":
		f.deleteRule(w, body)
	case "POST /api/v1/routing-rules/reorder":
		f.reorderRules(w, body)

	case "GET /api/v1/org/notification-order":
		writeJSON(w, f.notificationOrder)
	case "POST /api/v1/org/notification-order":
		f.notificationOrder = body
		writeJSON(w, body)

	case "POST /api/v1/org/members/list":
		writePage(w, []any{map[string]any{"user_id": fakeUserID, "email": fakeEmail, "display_name": "On Call", "role": "OWNER"}})

	default:
		http.NotFound(w, r)
	}
}

func (f *fakeAPI) create(w http.ResponseWriter, store map[string]map[string]any, body map[string]any, view func(map[string]any) map[string]any) {
	f.seq++
	id := fmt.Sprintf("00000000-0000-0000-0000-%012d", f.seq)
	body["id"] = id
	store[id] = view(body)
	writeJSON(w, store[id])
}

func (f *fakeAPI) get(w http.ResponseWriter, store map[string]map[string]any, id string) {
	obj, ok := store[id]
	if !ok {
		http.NotFound(w, nil)
		return
	}
	writeJSON(w, obj)
}

func (f *fakeAPI) update(w http.ResponseWriter, store map[string]map[string]any, body map[string]any, view func(map[string]any) map[string]any) {
	id, _ := body["id"].(string)
	if _, ok := store[id]; !ok {
		http.NotFound(w, nil)
		return
	}
	store[id] = view(body)
	writeJSON(w, store[id])
}

func (f *fakeAPI) delete(w http.ResponseWriter, store map[string]map[string]any, body map[string]any) {
	id, _ := body["id"].(string)
	delete(store, id)
	writeJSON(w, map[string]any{"id": id})
}

// Request uses policy_id; response nests it as policy{id,name}.
func (f *fakeAPI) serviceView(body map[string]any) map[string]any {
	pid, _ := body["policy_id"].(string)
	name := ""
	if p, ok := f.policies[pid]; ok {
		name, _ = p["name"].(string)
	}
	return map[string]any{
		"id":          body["id"],
		"name":        body["name"],
		"description": body["description"],
		"policy":      map[string]any{"id": pid, "name": name},
	}
}

func policyView(body map[string]any) map[string]any {
	if levels, ok := body["levels"].([]any); ok {
		for _, l := range levels {
			setDefault(l.(map[string]any), "timeout_s", 300)
		}
	}
	setDefault(body, "repeat_count", 0)
	setDefault(body, "ack_timeout_s", 1800)
	setDefault(body, "repage_after_ack", false)
	body["is_default"] = false
	return body
}

func scheduleView(body map[string]any) map[string]any {
	setDefault(body, "time_zone", "Asia/Saigon")
	body["rotation_start_at"] = toUTC(body["rotation_start_at"])
	for _, l := range body["layers"].([]any) {
		layer := l.(map[string]any)
		setDefault(layer, "time_zone", body["time_zone"])
		layer["rotation_start_at"] = toUTC(layer["rotation_start_at"])
		omitEmpty(layer, "restrictions", "participant_groups", "participant_user_ids")
	}
	return body
}

func (f *fakeAPI) addTeamMembers(w http.ResponseWriter, body map[string]any) {
	t, ok := f.teams[body["team_id"].(string)]
	if !ok {
		http.NotFound(w, nil)
		return
	}
	for _, u := range body["user_ids"].([]any) {
		t["members"] = append(t["members"].([]any), map[string]any{"user_id": u, "status": "active"})
	}
	writeJSON(w, t)
}

func (f *fakeAPI) removeTeamMember(w http.ResponseWriter, body map[string]any) {
	t, ok := f.teams[body["team_id"].(string)]
	if !ok {
		http.NotFound(w, nil)
		return
	}
	kept := []any{}
	for _, m := range t["members"].([]any) {
		if m.(map[string]any)["user_id"] != body["user_id"] {
			kept = append(kept, m)
		}
	}
	t["members"] = kept
	writeJSON(w, map[string]any{})
}

func (f *fakeAPI) ruleView(i int) map[string]any {
	r := f.rules[i]
	matcher, _ := r["matcher"].(map[string]any)
	if matcher == nil {
		matcher = map[string]any{}
	}
	enabled, ok := r["enabled"].(bool)
	return map[string]any{
		"id":       r["id"],
		"position": i,
		"matcher":  matcher,
		"policy":   map[string]any{"id": r["policy_id"], "name": ""},
		"enabled":  enabled || !ok,
	}
}

func (f *fakeAPI) ruleViews() []any {
	out := []any{}
	for i := range f.rules {
		out = append(out, f.ruleView(i))
	}
	return out
}

func (f *fakeAPI) updateRule(w http.ResponseWriter, body map[string]any) {
	for i, r := range f.rules {
		if r["id"] == body["id"] {
			f.rules[i] = body
			writeJSON(w, f.ruleView(i))
			return
		}
	}
	http.NotFound(w, nil)
}

func (f *fakeAPI) deleteRule(w http.ResponseWriter, body map[string]any) {
	for i, r := range f.rules {
		if r["id"] == body["id"] {
			f.rules = append(f.rules[:i], f.rules[i+1:]...)
			break
		}
	}
	writeJSON(w, map[string]any{"id": body["id"]})
}

func (f *fakeAPI) reorderRules(w http.ResponseWriter, body map[string]any) {
	byID := map[any]map[string]any{}
	for _, r := range f.rules {
		byID[r["id"]] = r
	}
	ordered := []map[string]any{}
	for _, id := range body["ids"].([]any) {
		ordered = append(ordered, byID[id])
	}
	f.rules = ordered
	writeJSON(w, f.ruleViews())
}

func toUTC(v any) any {
	s, _ := v.(string)
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return v
	}
	return t.UTC().Format(time.RFC3339)
}

func omitEmpty(m map[string]any, keys ...string) {
	for _, k := range keys {
		if a, ok := m[k].([]any); ok && len(a) == 0 {
			delete(m, k)
		}
	}
}

func setDefault(m map[string]any, k string, v any) {
	if m[k] == nil {
		m[k] = v
	}
}

func identity(m map[string]any) map[string]any { return m }

func sortedValues(store map[string]map[string]any) []any {
	ids := make([]string, 0, len(store))
	for id := range store {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := []any{}
	for _, id := range ids {
		out = append(out, store[id])
	}
	return out
}

func writePage(w http.ResponseWriter, items []any) {
	writeJSON(w, map[string]any{"total": len(items), "current_offset": 0, "data": items})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": v})
}

func testProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"opstracer": providerserver.NewProtocol6WithError(New("test")()),
	}
}

func providerConfig(url string) string {
	return fmt.Sprintf(`
provider "opstracer" {
  api_url = %q
  token   = %q
}
`, url, fakeToken)
}
