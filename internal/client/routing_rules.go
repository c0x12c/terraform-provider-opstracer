package client

import (
	"context"
	"net/http"
	"sort"
)

type RoutingRule struct {
	ID       string    `json:"id"`
	Position int64     `json:"position"`
	Matcher  Matcher   `json:"matcher"`
	Policy   PolicyRef `json:"policy"`
	Enabled  bool      `json:"enabled"`
}

// Matcher fields are ANDed; an empty matcher matches every alert.
type Matcher struct {
	Severity      []string          `json:"severity,omitempty"`
	SourceType    *string           `json:"source_type,omitempty"`
	IntegrationID *string           `json:"integration_id,omitempty"`
	Tags          map[string]string `json:"tags,omitempty"`
	RoutingHints  map[string]string `json:"routing_hints,omitempty"`
}

type RoutingRuleInput struct {
	ID       string  `json:"id,omitempty"`
	PolicyID string  `json:"policy_id"`
	Matcher  Matcher `json:"matcher"`
	Enabled  bool    `json:"enabled"`
}

// ListRoutingRules returns every rule in evaluation order.
func (c *Client) ListRoutingRules(ctx context.Context) ([]RoutingRule, error) {
	rules, err := listAll[RoutingRule](ctx, c, "/api/v1/routing-rules/list")
	if err != nil {
		return nil, err
	}
	sort.SliceStable(rules, func(i, j int) bool { return rules[i].Position < rules[j].Position })
	return rules, nil
}

func (c *Client) CreateRoutingRule(ctx context.Context, in RoutingRuleInput) (*RoutingRule, error) {
	var out RoutingRule
	return &out, c.do(ctx, http.MethodPost, "/api/v1/routing-rules", nil, in, &out)
}

func (c *Client) UpdateRoutingRule(ctx context.Context, in RoutingRuleInput) (*RoutingRule, error) {
	var out RoutingRule
	return &out, c.do(ctx, http.MethodPost, "/api/v1/routing-rules/update", nil, in, &out)
}

func (c *Client) DeleteRoutingRule(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/routing-rules/delete", nil, idRequest{ID: id}, nil)
}

func (c *Client) ReorderRoutingRules(ctx context.Context, ids []string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/routing-rules/reorder", nil, struct {
		IDs []string `json:"ids"`
	}{ids}, nil)
}
