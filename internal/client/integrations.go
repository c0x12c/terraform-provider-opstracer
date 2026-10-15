package client

import (
	"context"
	"net/http"
	"net/url"
)

type Integration struct {
	ID              string            `json:"id"`
	ServiceID       string            `json:"service_id"`
	Name            string            `json:"name"`
	Type            string            `json:"type"`
	Status          string            `json:"status"`
	TokenHint       string            `json:"token_hint"`
	IntegrationURL  string            `json:"integration_url"`
	DefaultSeverity string            `json:"default_severity"`
	SeverityMapping map[string]string `json:"severity_mapping"`
	RateSoftPerMin  int64             `json:"rate_soft_per_min"`
	RateHardPerMin  int64             `json:"rate_hard_per_min"`
	RotationPending bool              `json:"rotation_pending"`
}

type integrationWithKey struct {
	RoutingKey  string      `json:"routing_key"`
	Integration Integration `json:"integration"`
}

type IntegrationInput struct {
	ServiceID       string  `json:"service_id"`
	Name            string  `json:"name"`
	Type            string  `json:"type"`
	DefaultSeverity *string `json:"default_severity"`
	RateSoftPerMin  *int64  `json:"rate_soft_per_min"`
	RateHardPerMin  *int64  `json:"rate_hard_per_min"`
}

// IntegrationUpdate is a partial update: null fields are left unchanged, an empty mapping clears it.
type IntegrationUpdate struct {
	ID              string            `json:"id"`
	Name            *string           `json:"name"`
	Status          *string           `json:"status"`
	DefaultSeverity *string           `json:"default_severity"`
	SeverityMapping map[string]string `json:"severity_mapping"`
	RateSoftPerMin  *int64            `json:"rate_soft_per_min"`
	RateHardPerMin  *int64            `json:"rate_hard_per_min"`
}

// CreateIntegration returns the integration and its routing key, which the API never returns again.
func (c *Client) CreateIntegration(ctx context.Context, in IntegrationInput) (*Integration, string, error) {
	var out integrationWithKey
	err := c.do(ctx, http.MethodPost, "/api/v1/integrations", nil, in, &out)
	return &out.Integration, out.RoutingKey, err
}

func (c *Client) GetIntegration(ctx context.Context, id string) (*Integration, error) {
	var out Integration
	return &out, c.do(ctx, http.MethodGet, "/api/v1/integration", url.Values{"id": {id}}, nil, &out)
}

func (c *Client) UpdateIntegration(ctx context.Context, in IntegrationUpdate) (*Integration, error) {
	var out Integration
	return &out, c.do(ctx, http.MethodPost, "/api/v1/integrations/update", nil, in, &out)
}

func (c *Client) DeleteIntegration(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/integrations/delete", nil, idRequest{ID: id}, nil)
}

// RotateIntegrationKey issues a new routing key and retires the old one immediately.
func (c *Client) RotateIntegrationKey(ctx context.Context, id string) (*Integration, string, error) {
	var out integrationWithKey
	if err := c.do(ctx, http.MethodPost, "/api/v1/integrations/rotate-key", nil, idRequest{ID: id}, &out); err != nil {
		return nil, "", err
	}
	var retired Integration
	err := c.do(ctx, http.MethodPost, "/api/v1/integrations/retire-key", nil, idRequest{ID: id}, &retired)
	return &retired, out.RoutingKey, err
}
