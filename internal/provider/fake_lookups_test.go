package provider

import (
	"encoding/json"
	"net/http"
	"time"
)

const fakeDefaultPolicyID = "00000000-0000-0000-0000-00000000def0"

func init() {
	fakeRoutes["POST /api/v1/investigation/playbook-flows"] = func(f *fakeAPI, w http.ResponseWriter, _ *http.Request, body map[string]any) {
		if !validFlow(w, body) {
			return
		}
		body["version"] = 1
		f.create(w, f.store("playbook_flows"), body, flowView)
	}
	fakeRoutes["GET /api/v1/investigation/playbook-flow"] = func(f *fakeAPI, w http.ResponseWriter, r *http.Request, _ map[string]any) {
		flow, ok := f.store("playbook_flows")[r.URL.Query().Get("id")]
		if !ok {
			writeError(w, http.StatusNotFound, "playbook_flow_not_found")
			return
		}
		writeJSON(w, flow)
	}
	fakeRoutes["POST /api/v1/investigation/playbook-flow/update"] = func(f *fakeAPI, w http.ResponseWriter, _ *http.Request, body map[string]any) {
		flows := f.store("playbook_flows")
		old, ok := flows[body["id"].(string)]
		if !ok {
			writeError(w, http.StatusNotFound, "playbook_flow_not_found")
			return
		}
		if !validFlow(w, body) {
			return
		}
		if body["version"] != float64(old["version"].(int)) {
			writeError(w, http.StatusConflict, "playbook_flow_stale")
			return
		}
		body["version"] = old["version"].(int) + 1
		f.update(w, flows, body, flowView)
	}
	fakeRoutes["POST /api/v1/investigation/playbook-flow/delete"] = func(f *fakeAPI, w http.ResponseWriter, _ *http.Request, body map[string]any) {
		flows := f.store("playbook_flows")
		id, _ := body["id"].(string)
		if _, ok := flows[id]; !ok {
			writeError(w, http.StatusNotFound, "playbook_flow_not_found")
			return
		}
		delete(flows, id)
		w.WriteHeader(http.StatusOK)
	}
	fakeRoutes["POST /api/v1/investigation/playbook-flows/list"] = func(f *fakeAPI, w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		items := []any{}
		for _, v := range sortedValues(f.store("playbook_flows")) {
			item := copyWithout(v.(map[string]any), "graph")
			item["playbook_count"] = 0
			items = append(items, item)
		}
		writePage(w, items)
	}

	fakeRoutes["POST /api/v1/services/list"] = func(f *fakeAPI, w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		items := []any{}
		for _, v := range sortedValues(f.services) {
			item := copyWithout(v.(map[string]any))
			if item["description"] == nil {
				delete(item, "description")
			}
			item["integration_count"] = 0
			items = append(items, item)
		}
		writePage(w, items)
	}
	fakeRoutes["POST /api/v1/escalation-policies/list"] = func(f *fakeAPI, w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		f.ensureDefaultPolicy()
		items := []any{}
		for _, v := range sortedValues(f.policies) {
			p := v.(map[string]any)
			item := copyWithout(p, "levels")
			levels, _ := p["levels"].([]any)
			item["level_count"] = len(levels)
			items = append(items, item)
		}
		writePage(w, items)
	}
	fakeRoutes["GET /api/v1/escalation-policies/default"] = func(f *fakeAPI, w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		f.ensureDefaultPolicy()
		for _, p := range f.policies {
			if p["is_default"] == true {
				writeJSON(w, p)
				return
			}
		}
	}
	fakeRoutes["POST /api/v1/org/teams/list"] = func(f *fakeAPI, w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		items := []any{}
		for _, v := range sortedValues(f.teams) {
			item := copyWithout(v.(map[string]any))
			item["is_default"] = false
			item["member_count"] = len(item["members"].([]any))
			items = append(items, item)
		}
		writePage(w, items)
	}
	fakeRoutes["POST /api/v1/schedules/list"] = func(f *fakeAPI, w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		writePage(w, sortedValues(f.schedules))
	}
	fakeRoutes["GET /api/v1/schedule/on-call"] = func(f *fakeAPI, w http.ResponseWriter, r *http.Request, _ map[string]any) {
		s, ok := f.schedules[r.URL.Query().Get("id")]
		if !ok {
			writeError(w, http.StatusNotFound, "schedule_not_found")
			return
		}
		at := time.Now().UTC()
		if q := r.URL.Query().Get("at"); q != "" {
			var err error
			if at, err = time.Parse(time.RFC3339, q); err != nil {
				http.Error(w, "Bad Request", http.StatusBadRequest)
				return
			}
		}
		writeJSON(w, onCallAt(s, at.UTC()))
	}
}

// onCallAt walks the first layer's rotation; before the rotation starts nobody is on call and
// user_ids is omitted, as on dev.
func onCallAt(s map[string]any, at time.Time) map[string]any {
	out := map[string]any{"schedule_id": s["id"], "at": at.Format(time.RFC3339Nano)}
	layer := s["layers"].([]any)[0].(map[string]any)
	start, _ := time.Parse(time.RFC3339, layer["rotation_start_at"].(string))
	period := time.Duration(layer["member_period_seconds"].(float64)) * time.Second
	if at.Before(start) {
		out["until"] = start.Format(time.RFC3339)
		return out
	}
	groups, _ := layer["participant_groups"].([]any)
	if len(groups) == 0 {
		return out
	}
	n := int(at.Sub(start) / period)
	holders := groups[n%len(groups)].([]any)
	out["user_id"] = holders[0]
	out["user_ids"] = holders
	out["until"] = start.Add(time.Duration(n+1) * period).Format(time.RFC3339)
	return out
}

// The real org always has a default policy, created with the org.
func (f *fakeAPI) ensureDefaultPolicy() {
	for _, p := range f.policies {
		if p["is_default"] == true {
			return
		}
	}
	f.policies[fakeDefaultPolicyID] = map[string]any{
		"id": fakeDefaultPolicyID, "name": "Default policy", "description": "Created with the organization.",
		"levels":       []any{map[string]any{"targets": []any{map[string]any{"type": "user", "user_id": fakeUserID}}, "timeout_s": 300}},
		"repeat_count": 0, "ack_timeout_s": 3600, "repage_after_ack": false, "is_default": true,
	}
}

// validFlow mirrors dev: unknown enum values fail parsing with a bare Bad Request, and the
// graph needs exactly one incident node.
func validFlow(w http.ResponseWriter, body map[string]any) bool {
	if v := body["visibility"]; v != "org" && v != "private" {
		writeError(w, http.StatusBadRequest, "error")
		return false
	}
	graph, _ := body["graph"].(map[string]any)
	nodes, _ := graph["nodes"].([]any)
	incidents := 0
	for _, n := range nodes {
		switch n.(map[string]any)["type"] {
		case "incident":
			incidents++
		case "playbook", "condition", "end", "topology":
		default:
			writeError(w, http.StatusBadRequest, "error")
			return false
		}
	}
	if incidents != 1 {
		writeError(w, http.StatusBadRequest, "playbook_flow_invalid")
		return false
	}
	return true
}

// flowView fills the node fields dev always returns and normalises the body via a JSON round trip.
func flowView(body map[string]any) map[string]any {
	graph := body["graph"].(map[string]any)
	for _, n := range graph["nodes"].([]any) {
		node := n.(map[string]any)
		setDefault(node, "playbook", nil)
		setDefault(node, "values", map[string]any{})
		setDefault(node, "entities", []any{})
		setDefault(node, "addons", map[string]any{})
	}
	setDefault(graph, "edges", []any{})
	raw, _ := json.Marshal(graph)
	var g map[string]any
	_ = json.Unmarshal(raw, &g)
	setDefault(body, "description", nil)
	body["graph"] = g
	body["created_by"] = fakeUserID
	body["updated_by"] = fakeUserID
	return body
}

func copyWithout(m map[string]any, drop ...string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	for _, k := range drop {
		delete(out, k)
	}
	return out
}
