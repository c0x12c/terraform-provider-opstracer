package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Integrations, observability connections and service data sources, as seen on dev.
func init() {
	fakeRoutes["POST /api/v1/integrations"] = (*fakeAPI).createIntegration
	fakeRoutes["GET /api/v1/integration"] = func(f *fakeAPI, w http.ResponseWriter, r *http.Request, _ map[string]any) {
		f.get(w, f.store("integrations"), r.URL.Query().Get("id"))
	}
	fakeRoutes["POST /api/v1/integrations/list"] = func(f *fakeAPI, w http.ResponseWriter, _ *http.Request, _ map[string]any) {
		writePage(w, sortedValues(f.store("integrations")))
	}
	fakeRoutes["POST /api/v1/integrations/update"] = (*fakeAPI).updateIntegration
	fakeRoutes["POST /api/v1/integrations/delete"] = func(f *fakeAPI, w http.ResponseWriter, _ *http.Request, body map[string]any) {
		if _, ok := f.store("integrations")[body["id"].(string)]; !ok {
			writeAPIError(w, http.StatusNotFound, "not_found", "Resource not found")
			return
		}
		f.delete(w, f.store("integrations"), body)
	}
	fakeRoutes["POST /api/v1/integrations/rotate-key"] = (*fakeAPI).rotateIntegrationKey
	fakeRoutes["POST /api/v1/integrations/retire-key"] = func(f *fakeAPI, w http.ResponseWriter, _ *http.Request, body map[string]any) {
		i, ok := f.store("integrations")[body["id"].(string)]
		if !ok {
			writeAPIError(w, http.StatusNotFound, "not_found", "Resource not found")
			return
		}
		i["rotation_pending"] = false
		writeJSON(w, i)
	}

	fakeRoutes["POST /api/v1/observability/connections"] = (*fakeAPI).createConnection
	fakeRoutes["POST /api/v1/observability/connections/list"] = (*fakeAPI).listConnections
	fakeRoutes["POST /api/v1/observability/connections/update"] = func(f *fakeAPI, w http.ResponseWriter, _ *http.Request, body map[string]any) {
		c, ok := f.store("connections")[body["id"].(string)]
		if !ok {
			writeAPIError(w, http.StatusBadRequest, "observability_invalid_request", fmt.Sprintf("connection %s not found", body["id"]))
			return
		}
		// Only the name changes; everything else in the body is ignored and the reply is empty.
		c["name"] = body["name"]
	}
	fakeRoutes["POST /api/v1/observability/connections/revoke"] = func(f *fakeAPI, w http.ResponseWriter, _ *http.Request, body map[string]any) {
		id, _ := body["id"].(string)
		if _, ok := f.store("connections")[id]; !ok {
			writeAPIError(w, http.StatusBadRequest, "observability_invalid_request", fmt.Sprintf("connection %s not found", id))
			return
		}
		delete(f.store("connections"), id)
	}

	fakeRoutes["GET /api/v1/service/data-sources"] = (*fakeAPI).getServiceDataSources
	fakeRoutes["POST /api/v1/service/data-sources/update"] = (*fakeAPI).updateServiceDataSource
}

var fakeConnectionTypes = map[string]struct {
	display  string
	required []string
}{
	"datadog":    {"Datadog", []string{"api_key"}},
	"grafana":    {"Grafana", []string{"token", "site"}},
	"sentry":     {"Sentry", []string{"token", "site"}},
	"newrelic":   {"New Relic", []string{"api_key"}},
	"cloudwatch": {"CloudWatch", nil},
	"gcp":        {"Google Cloud", []string{"service_account_key"}},
}

func writeAPIError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": code, "message": msg}})
}

func last4(s string) string {
	if len(s) < 4 {
		return s
	}
	return s[len(s)-4:]
}

func (f *fakeAPI) createIntegration(w http.ResponseWriter, _ *http.Request, body map[string]any) {
	switch body["type"] {
	case "generic", "datadog", "grafana", "sentry", "newrelic", "cloudwatch", "prometheus":
	default:
		writeAPIError(w, http.StatusBadRequest, "validation_error", "Validation failed")
		return
	}
	f.seq++
	id := fmt.Sprintf("00000000-0000-0000-0000-%012d", f.seq)
	serviceID, _ := body["service_id"].(string)
	if serviceID == "" {
		// Dev creates a service named after the integration and leaves it behind on delete.
		serviceID = fmt.Sprintf("00000000-0000-0000-0001-%012d", f.seq)
		f.services[serviceID] = map[string]any{"id": serviceID, "name": body["name"], "policy": map[string]any{"id": "", "name": "Default policy"}}
	}
	key := fmt.Sprintf("opst_fakekey%04d", f.seq)
	i := map[string]any{
		"id":                 id,
		"service_id":         serviceID,
		"service_name":       f.services[serviceID]["name"],
		"name":               body["name"],
		"type":               body["type"],
		"status":             "active",
		"token_hint":         last4(key),
		"integration_url":    "https://api-ingestion.example.test/v1/events",
		"default_severity":   "P3",
		"severity_mapping":   nil,
		"rate_soft_per_min":  600,
		"rate_hard_per_min":  6000,
		"token_rotated_at":   nil,
		"rotation_pending":   false,
		"created_at":         time.Now().UTC().Format(time.RFC3339Nano),
		"updated_at":         nil,
		"first_event_at":     nil,
		"vercel_config":      nil,
		"vercel_webhook_url": nil,
	}
	for _, k := range []string{"default_severity", "rate_soft_per_min", "rate_hard_per_min"} {
		if body[k] != nil {
			i[k] = body[k]
		}
	}
	f.store("integrations")[id] = i
	writeJSON(w, map[string]any{"routing_key": key, "integration": i})
}

// Update is partial: null fields are left alone, an empty severity_mapping is stored as {}.
func (f *fakeAPI) updateIntegration(w http.ResponseWriter, _ *http.Request, body map[string]any) {
	i, ok := f.store("integrations")[body["id"].(string)]
	if !ok {
		writeAPIError(w, http.StatusNotFound, "not_found", "Resource not found")
		return
	}
	if s, ok := body["status"].(string); ok && s != "active" && s != "disabled" {
		writeAPIError(w, http.StatusBadRequest, "validation_error", "Validation failed")
		return
	}
	soft, hard := body["rate_soft_per_min"], body["rate_hard_per_min"]
	if soft == nil {
		soft = i["rate_soft_per_min"]
	}
	if hard == nil {
		hard = i["rate_hard_per_min"]
	}
	if toFloat(soft) > toFloat(hard) {
		writeAPIError(w, http.StatusBadRequest, "validation_error", "Validation failed")
		return
	}
	for _, k := range []string{"name", "status", "default_severity", "severity_mapping", "rate_soft_per_min", "rate_hard_per_min"} {
		if body[k] != nil {
			i[k] = body[k]
		}
	}
	i["updated_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	writeJSON(w, i)
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	}
	return 0
}

func (f *fakeAPI) rotateIntegrationKey(w http.ResponseWriter, _ *http.Request, body map[string]any) {
	i, ok := f.store("integrations")[body["id"].(string)]
	if !ok {
		writeAPIError(w, http.StatusNotFound, "not_found", "Resource not found")
		return
	}
	f.seq++
	key := fmt.Sprintf("opst_rotatedkey%04d", f.seq)
	i["token_hint"] = last4(key)
	i["rotation_pending"] = true
	i["token_rotated_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	writeJSON(w, map[string]any{"routing_key": key, "integration": i})
}

func (f *fakeAPI) createConnection(w http.ResponseWriter, _ *http.Request, body map[string]any) {
	typ, _ := body["type"].(string)
	spec, ok := fakeConnectionTypes[typ]
	if !ok {
		writeAPIError(w, http.StatusBadRequest, "observability_invalid_request", fmt.Sprintf("'%s' is not an observability provider", typ))
		return
	}
	hints := map[string]any{}
	for _, k := range spec.required {
		v, _ := body[k].(string)
		if v == "" {
			writeAPIError(w, http.StatusBadRequest, "observability_invalid_request", fmt.Sprintf("a %s connection needs %s", typ, k))
			return
		}
		if k != "site" {
			hints[k] = last4(v)
		}
	}
	isDefault := true
	for _, c := range f.store("connections") {
		if c["type"] == typ {
			isDefault = false
		}
	}
	name, _ := body["name"].(string)
	if name == "" {
		name = spec.display
	}
	f.seq++
	id := fmt.Sprintf("00000000-0000-0000-0002-%012d", f.seq)
	c := map[string]any{
		"id":               id,
		"type":             typ,
		"name":             name,
		"is_default":       isDefault,
		"site":             body["site"],
		"status":           "active",
		"capabilities":     []any{"monitors"},
		"credential_hints": hints,
		"last_probe_at":    time.Now().UTC().Format(time.RFC3339Nano),
		// env_label is accepted but ignored for grafana on dev.
		"settings": map[string]any{"env_label": "env", "service_label": "service_name", "metric_env_label": "env", "metric_service_label": "service_name"},
	}
	f.store("connections")[id] = c
	// service_ids pins the new connection on those services.
	if ids, ok := body["service_ids"].([]any); ok {
		for _, s := range ids {
			f.serviceOverrides(s.(string))[typ] = map[string]any{"choice": "connection", "connection_id": id}
		}
	}
	writeJSON(w, c)
}

func (f *fakeAPI) listConnections(w http.ResponseWriter, _ *http.Request, body map[string]any) {
	out := []any{}
	for _, c := range sortedValues(f.store("connections")) {
		if t, ok := body["type"].(string); !ok || c.(map[string]any)["type"] == t {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		writeJSON(w, map[string]any{})
		return
	}
	writeJSON(w, map[string]any{"connections": out})
}

func (f *fakeAPI) serviceOverrides(serviceID string) map[string]any {
	s := f.store("service_data_sources")
	if s[serviceID] == nil {
		s[serviceID] = map[string]any{}
	}
	return s[serviceID]
}

func (f *fakeAPI) dataSourceRow(serviceID, typ string) map[string]any {
	row := map[string]any{"type": typ, "choice": "default", "connection_id": nil, "reads": nil, "unavailable": nil}
	if o, ok := f.serviceOverrides(serviceID)[typ].(map[string]any); ok {
		row["choice"], row["connection_id"] = o["choice"], o["connection_id"]
	}
	var options []any
	for _, c := range sortedValues(f.store("connections")) {
		conn := c.(map[string]any)
		if conn["type"] != typ {
			continue
		}
		opt := map[string]any{"id": conn["id"], "name": conn["name"], "is_default": conn["is_default"], "status": conn["status"]}
		options = append(options, opt)
		if (row["choice"] == "default" && conn["is_default"] == true) || (row["choice"] == "connection" && row["connection_id"] == conn["id"]) {
			row["reads"] = opt
		}
	}
	if len(options) > 0 {
		row["options"] = options
	}
	if row["reads"] == nil && row["choice"] != "off" {
		row["unavailable"] = "not_connected"
	}
	return row
}

// A provider is listed when the service overrides it or the org has a connection for it.
func (f *fakeAPI) getServiceDataSources(w http.ResponseWriter, r *http.Request, _ map[string]any) {
	serviceID := r.URL.Query().Get("service_id")
	if _, ok := f.services[serviceID]; !ok {
		writeAPIError(w, http.StatusBadRequest, "observability_invalid_request", fmt.Sprintf("service %s not found", serviceID))
		return
	}
	types := map[string]bool{}
	for typ := range f.serviceOverrides(serviceID) {
		types[typ] = true
	}
	for _, c := range f.store("connections") {
		types[c["type"].(string)] = true
	}
	var sources []any
	for _, typ := range []string{"cloudwatch", "datadog", "gcp", "grafana", "newrelic", "railway", "sentry"} {
		if types[typ] {
			sources = append(sources, f.dataSourceRow(serviceID, typ))
		}
	}
	if len(sources) == 0 {
		writeJSON(w, map[string]any{})
		return
	}
	writeJSON(w, map[string]any{"sources": sources})
}

func (f *fakeAPI) updateServiceDataSource(w http.ResponseWriter, _ *http.Request, body map[string]any) {
	serviceID, _ := body["service_id"].(string)
	typ, _ := body["type"].(string)
	choice, _ := body["choice"].(string)
	if _, ok := f.services[serviceID]; !ok {
		writeAPIError(w, http.StatusBadRequest, "observability_invalid_request", fmt.Sprintf("service %s not found", serviceID))
		return
	}
	if _, ok := fakeConnectionTypes[typ]; !ok && typ != "railway" {
		writeAPIError(w, http.StatusBadRequest, "observability_invalid_request", typ+" is not a data source a service can choose")
		return
	}
	switch choice {
	case "default":
		delete(f.serviceOverrides(serviceID), typ)
	case "off":
		f.serviceOverrides(serviceID)[typ] = map[string]any{"choice": "off", "connection_id": nil}
	case "connection":
		if body["connection_id"] == nil {
			writeAPIError(w, http.StatusBadRequest, "observability_invalid_request", "connection_id is required to choose a connection")
			return
		}
		f.serviceOverrides(serviceID)[typ] = map[string]any{"choice": "connection", "connection_id": body["connection_id"]}
	default:
		writeAPIError(w, http.StatusBadRequest, "observability_invalid_request", "choice must be default, connection or off")
		return
	}
	writeJSON(w, f.dataSourceRow(serviceID, typ))
}
