package client

import (
	"context"
	"net/http"
	"net/url"
)

type Service struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	Policy      PolicyRef `json:"policy"`
}

type PolicyRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ServiceInput struct {
	ID          string  `json:"id,omitempty"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	PolicyID    string  `json:"policy_id"`
}

func (c *Client) CreateService(ctx context.Context, in ServiceInput) (*Service, error) {
	var out Service
	return &out, c.do(ctx, http.MethodPost, "/api/v1/services", nil, in, &out)
}

func (c *Client) GetService(ctx context.Context, id string) (*Service, error) {
	var out Service
	return &out, c.do(ctx, http.MethodGet, "/api/v1/service", url.Values{"id": {id}}, nil, &out)
}

func (c *Client) UpdateService(ctx context.Context, in ServiceInput) (*Service, error) {
	var out Service
	return &out, c.do(ctx, http.MethodPost, "/api/v1/services/update", nil, in, &out)
}

func (c *Client) DeleteService(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/services/delete", nil, idRequest{ID: id}, nil)
}

func (c *Client) ListServices(ctx context.Context) ([]Service, error) {
	return listAll[Service](ctx, c, "/api/v1/services/list")
}
