package client

import (
	"context"
	"encoding/json"
	"net/http"
)

// Suppression silences the alerts of one service that match Matcher, for a time window.
// Ended and expired suppressions drop out of the list.
type Suppression struct {
	ID         string             `json:"id"`
	ServiceID  string             `json:"service_id"`
	Matcher    SuppressionMatcher `json:"matcher"`
	StartsAt   string             `json:"starts_at"`
	EndsAt     string             `json:"ends_at"`
	Reason     *string            `json:"reason"`
	Active     bool               `json:"active"`
	MatchCount int64              `json:"match_count"`
}

// SuppressionMatcher has the routing rule conditions, but the server echoes severity back as sent,
// which may be a single string.
type SuppressionMatcher struct {
	Severity      StringList        `json:"severity,omitempty"`
	SourceType    *string           `json:"source_type,omitempty"`
	IntegrationID *string           `json:"integration_id,omitempty"`
	Tags          map[string]string `json:"tags,omitempty"`
	RoutingHints  map[string]string `json:"routing_hints,omitempty"`
}

// StringList decodes either a JSON string or a list of strings.
type StringList []string

func (l *StringList) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*l = StringList{s}
		return nil
	}
	return json.Unmarshal(b, (*[]string)(l))
}

type SuppressionInput struct {
	ServiceID string             `json:"service_id"`
	Matcher   SuppressionMatcher `json:"matcher"`
	StartsAt  *string            `json:"starts_at"`
	EndsAt    string             `json:"ends_at"`
	Reason    *string            `json:"reason"`
}

type suppressionListRequest struct {
	ServiceID string `json:"service_id"`
	listRequest
}

// ListSuppressions returns the scheduled and running suppressions of a service.
func (c *Client) ListSuppressions(ctx context.Context, serviceID string) ([]Suppression, error) {
	return listAllWith[Suppression](ctx, c, "/api/v1/suppressions/list", func(r listRequest) any {
		return suppressionListRequest{ServiceID: serviceID, listRequest: r}
	})
}

func (c *Client) CreateSuppression(ctx context.Context, in SuppressionInput) (*Suppression, error) {
	var out Suppression
	return &out, c.do(ctx, http.MethodPost, "/api/v1/suppressions", nil, in, &out)
}

// EndSuppression stops a running suppression now; the server rejects it before the start.
func (c *Client) EndSuppression(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/suppressions/end", nil, idRequest{ID: id}, nil)
}

// CancelSuppression removes a scheduled suppression; the server rejects it once it has started.
func (c *Client) CancelSuppression(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/suppressions/cancel", nil, idRequest{ID: id}, nil)
}
