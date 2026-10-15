package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

type PlaybookFlow struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description *string         `json:"description"`
	Visibility  string          `json:"visibility"`
	Version     int64           `json:"version"`
	Graph       json.RawMessage `json:"graph"`
}

// PlaybookFlowInput.Version must be the current version for updates; a stale one returns 409.
type PlaybookFlowInput struct {
	ID          string          `json:"id,omitempty"`
	Version     int64           `json:"version,omitempty"`
	Name        string          `json:"name"`
	Description *string         `json:"description"`
	Visibility  string          `json:"visibility"`
	Graph       json.RawMessage `json:"graph"`
}

func (c *Client) CreatePlaybookFlow(ctx context.Context, in PlaybookFlowInput) (*PlaybookFlow, error) {
	var out PlaybookFlow
	return &out, c.do(ctx, http.MethodPost, "/api/v1/investigation/playbook-flows", nil, in, &out)
}

func (c *Client) GetPlaybookFlow(ctx context.Context, id string) (*PlaybookFlow, error) {
	var out PlaybookFlow
	return &out, c.do(ctx, http.MethodGet, "/api/v1/investigation/playbook-flow", url.Values{"id": {id}}, nil, &out)
}

func (c *Client) UpdatePlaybookFlow(ctx context.Context, in PlaybookFlowInput) (*PlaybookFlow, error) {
	var out PlaybookFlow
	return &out, c.do(ctx, http.MethodPost, "/api/v1/investigation/playbook-flow/update", nil, in, &out)
}

func (c *Client) DeletePlaybookFlow(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/investigation/playbook-flow/delete", nil, idRequest{ID: id}, nil)
}
