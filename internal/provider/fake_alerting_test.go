package provider

import (
	"encoding/json"
	"net/http"
	"time"
)

// Mutes and suppressions as seen on dev: no get-by-id, lists only hold scheduled and running windows,
// end only works once started and delete/cancel only before, with 409 error codes otherwise.
func init() {
	fakeRoutes["POST /api/v1/mutes"] = func(f *fakeAPI, w http.ResponseWriter, _ *http.Request, body map[string]any) {
		scopeID, _ := body["scope_id"].(string)
		switch body["scope_type"] {
		case "service":
			if _, ok := f.services[scopeID]; !ok {
				writeError(w, http.StatusBadRequest, "validation_error")
				return
			}
			body["service_id"] = scopeID
			body["scope_name"] = f.services[scopeID]["name"]
		default:
			writeError(w, http.StatusBadRequest, "validation_error")
			return
		}
		if !f.newWindow(w, body) {
			return
		}
		if r, _ := body["reason"].(string); r == "" {
			delete(body, "reason")
		}
		f.create(w, f.store("mutes"), body, identity)
	}
	fakeRoutes["POST /api/v1/mutes/list"] = func(f *fakeAPI, w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		writePage(w, liveWindows(f.store("mutes"), nil))
	}
	fakeRoutes["POST /api/v1/mutes/end"] = func(f *fakeAPI, w http.ResponseWriter, _ *http.Request, body map[string]any) {
		endWindow(w, f.store("mutes"), body, "mute")
	}
	fakeRoutes["POST /api/v1/mutes/delete"] = func(f *fakeAPI, w http.ResponseWriter, _ *http.Request, body map[string]any) {
		removeWindow(w, f, f.store("mutes"), body, "mute", "mute_already_started")
	}

	fakeRoutes["POST /api/v1/suppressions"] = func(f *fakeAPI, w http.ResponseWriter, _ *http.Request, body map[string]any) {
		if _, ok := body["matcher"].(map[string]any); !ok {
			writeError(w, http.StatusBadRequest, "validation_error")
			return
		}
		if !f.newWindow(w, body) {
			return
		}
		if r, _ := body["reason"].(string); r == "" {
			body["reason"] = nil
		}
		body["ended_by"] = nil
		body["match_count"] = 0
		f.create(w, f.store("suppressions"), body, identity)
	}
	fakeRoutes["POST /api/v1/suppressions/list"] = func(f *fakeAPI, w http.ResponseWriter, _ *http.Request, body map[string]any) {
		serviceID, ok := body["service_id"].(string)
		if !ok {
			writeError(w, http.StatusBadRequest, "error")
			return
		}
		writePage(w, liveWindows(f.store("suppressions"), func(s map[string]any) bool { return s["service_id"] == serviceID }))
	}
	fakeRoutes["POST /api/v1/suppressions/end"] = func(f *fakeAPI, w http.ResponseWriter, _ *http.Request, body map[string]any) {
		endWindow(w, f.store("suppressions"), body, "suppression")
	}
	fakeRoutes["POST /api/v1/suppressions/cancel"] = func(f *fakeAPI, w http.ResponseWriter, _ *http.Request, body map[string]any) {
		removeWindow(w, f, f.store("suppressions"), body, "suppression", "suppression_already_started")
	}
}

// newWindow validates and normalises starts_at/ends_at like the server: UTC, start defaults to now.
func (f *fakeAPI) newWindow(w http.ResponseWriter, body map[string]any) bool {
	now := time.Now().UTC()
	start := now
	if s, ok := body["starts_at"].(string); ok {
		start, _ = time.Parse(time.RFC3339, s)
	}
	end, err := time.Parse(time.RFC3339, fmtString(body["ends_at"]))
	if err != nil || !end.After(start) || !end.After(now) {
		writeError(w, http.StatusBadRequest, "validation_error")
		return false
	}
	body["starts_at"] = start.UTC().Format(time.RFC3339Nano)
	body["ends_at"] = end.UTC().Format(time.RFC3339Nano)
	body["active"] = !start.After(now)
	body["created_by"] = fakeUserID
	body["created_at"] = now.Format(time.RFC3339Nano)
	return true
}

func liveWindows(store map[string]map[string]any, keep func(map[string]any) bool) []any {
	now := time.Now()
	out := []any{}
	for _, v := range sortedValues(store) {
		obj := v.(map[string]any)
		start, _ := time.Parse(time.RFC3339, obj["starts_at"].(string))
		end, _ := time.Parse(time.RFC3339, obj["ends_at"].(string))
		if !end.After(now) || (keep != nil && !keep(obj)) {
			continue
		}
		obj["active"] = !start.After(now)
		out = append(out, obj)
	}
	return out
}

func endWindow(w http.ResponseWriter, store map[string]map[string]any, body map[string]any, kind string) {
	obj, ok := store[fmtString(body["id"])]
	if !ok {
		writeError(w, http.StatusNotFound, kind+"_not_found")
		return
	}
	now := time.Now()
	start, _ := time.Parse(time.RFC3339, obj["starts_at"].(string))
	end, _ := time.Parse(time.RFC3339, obj["ends_at"].(string))
	switch {
	case !end.After(now):
		writeError(w, http.StatusConflict, kind+"_already_ended")
	case start.After(now):
		writeError(w, http.StatusConflict, kind+"_not_started")
	default:
		obj["ends_at"] = now.UTC().Format(time.RFC3339Nano)
		obj["ended_by"] = fakeUserID
		obj["active"] = false
		writeJSON(w, obj)
	}
}

func removeWindow(w http.ResponseWriter, f *fakeAPI, store map[string]map[string]any, body map[string]any, kind, startedCode string) {
	obj, ok := store[fmtString(body["id"])]
	if !ok {
		writeError(w, http.StatusNotFound, kind+"_not_found")
		return
	}
	start, _ := time.Parse(time.RFC3339, obj["starts_at"].(string))
	if !start.After(time.Now()) {
		writeError(w, http.StatusConflict, startedCode)
		return
	}
	f.delete(w, store, body)
}

func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": code, "message": code}})
}

func fmtString(v any) string {
	s, _ := v.(string)
	return s
}
