package client

import (
	"context"
	"encoding/json"
	"net/http"
)

// LogMonitor polls one log source (e.g. a Railway service) and evaluates its Rules.
type LogMonitor struct {
	ID            string          `json:"id"`
	IntegrationID string          `json:"integration_id"`
	SourceType    string          `json:"source_type"`
	Scope         json.RawMessage `json:"scope"`
	ScopeName     *string         `json:"scope_name"`
	Enabled       bool            `json:"enabled"`
	Rules         []LogAlertRule  `json:"rules"`
}

type LogMonitorInput struct {
	SourceType    string            `json:"source_type"`
	Scope         map[string]string `json:"scope"`
	IntegrationID string            `json:"integration_id"`
	ConnectionID  *string           `json:"connection_id"`
}

type LogAlertRule struct {
	ID            string   `json:"id"`
	MonitorID     string   `json:"monitor_id"`
	Name          string   `json:"name"`
	ServiceID     *string  `json:"service_id"`
	ServiceName   *string  `json:"service_name"`
	MatchLevel    *string  `json:"match_level"`
	MatchText     []string `json:"match_text"`
	Threshold     int64    `json:"threshold"`
	WindowMinutes int64    `json:"window_minutes"`
	Severity      string   `json:"severity"`
	Enabled       bool     `json:"enabled"`
	State         string   `json:"state"`
}

type LogAlertRuleInput struct {
	ID            string   `json:"id,omitempty"`
	MonitorID     string   `json:"monitor_id,omitempty"`
	Name          string   `json:"name"`
	ServiceID     *string  `json:"service_id"`
	ServiceName   *string  `json:"service_name"`
	MatchLevel    *string  `json:"match_level"`
	MatchText     []string `json:"match_text"`
	Threshold     int64    `json:"threshold"`
	WindowMinutes int64    `json:"window_minutes"`
	Severity      string   `json:"severity"`
	Enabled       bool     `json:"enabled"`
}

// ListLogMonitors returns every monitor with its rules. It is a plain GET, not a paged list.
func (c *Client) ListLogMonitors(ctx context.Context) ([]LogMonitor, error) {
	var out []LogMonitor
	return out, c.do(ctx, http.MethodGet, "/api/v1/log-alerts/monitors", nil, nil, &out)
}

func (c *Client) CreateLogMonitor(ctx context.Context, in LogMonitorInput) (*LogMonitor, error) {
	var out LogMonitor
	return &out, c.do(ctx, http.MethodPost, "/api/v1/log-alerts/monitors", nil, in, &out)
}

func (c *Client) SetLogMonitorEnabled(ctx context.Context, id string, enabled bool) (*LogMonitor, error) {
	path := "/api/v1/log-alerts/monitors/disable"
	if enabled {
		path = "/api/v1/log-alerts/monitors/enable"
	}
	var out LogMonitor
	return &out, c.do(ctx, http.MethodPost, path, nil, idRequest{ID: id}, &out)
}

func (c *Client) DeleteLogMonitor(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/log-alerts/monitors/delete", nil, idRequest{ID: id}, nil)
}

func (c *Client) CreateLogAlertRule(ctx context.Context, in LogAlertRuleInput) (*LogAlertRule, error) {
	var out LogAlertRule
	return &out, c.do(ctx, http.MethodPost, "/api/v1/log-alerts/rules", nil, in, &out)
}

func (c *Client) UpdateLogAlertRule(ctx context.Context, in LogAlertRuleInput) (*LogAlertRule, error) {
	var out LogAlertRule
	return &out, c.do(ctx, http.MethodPost, "/api/v1/log-alerts/rules/update", nil, in, &out)
}

func (c *Client) DeleteLogAlertRule(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/log-alerts/rules/delete", nil, idRequest{ID: id}, nil)
}
