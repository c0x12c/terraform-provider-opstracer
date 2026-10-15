package client

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

type ObservabilityConnection struct {
	ID              string            `json:"id"`
	Type            string            `json:"type"`
	Name            string            `json:"name"`
	IsDefault       bool              `json:"is_default"`
	Site            *string           `json:"site"`
	Status          string            `json:"status"`
	CredentialHints map[string]string `json:"credential_hints"`
	Settings        map[string]string `json:"settings"`
}

type ObservabilityConnectionInput struct {
	Type              string  `json:"type"`
	Name              *string `json:"name,omitempty"`
	Site              *string `json:"site,omitempty"`
	APIKey            *string `json:"api_key,omitempty"`
	ApplicationKey    *string `json:"application_key,omitempty"`
	Token             *string `json:"token,omitempty"`
	Organization      *string `json:"organization,omitempty"`
	AccountID         *string `json:"account_id,omitempty"`
	RoleARN           *string `json:"role_arn,omitempty"`
	AccessKeyID       *string `json:"access_key_id,omitempty"`
	SecretAccessKey   *string `json:"secret_access_key,omitempty"`
	ServiceAccountKey *string `json:"service_account_key,omitempty"`
	EnvLabel          *string `json:"env_label,omitempty"`
}

func (c *Client) CreateObservabilityConnection(ctx context.Context, in ObservabilityConnectionInput) (*ObservabilityConnection, error) {
	var out ObservabilityConnection
	return &out, c.do(ctx, http.MethodPost, "/api/v1/observability/connections", nil, in, &out)
}

// GetObservabilityConnection scans the list: there is no get-by-id, and revoked connections drop out of it.
func (c *Client) GetObservabilityConnection(ctx context.Context, id string) (*ObservabilityConnection, error) {
	var out struct {
		Connections []ObservabilityConnection `json:"connections"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/v1/observability/connections/list", nil, struct{}{}, &out); err != nil {
		return nil, err
	}
	for _, conn := range out.Connections {
		if conn.ID == id {
			return &conn, nil
		}
	}
	return nil, &APIError{Method: http.MethodPost, Path: "/api/v1/observability/connections/list", StatusCode: http.StatusNotFound, Body: "connection " + id + " not found"}
}

// RenameObservabilityConnection is the only update the API supports; it returns an empty body.
func (c *Client) RenameObservabilityConnection(ctx context.Context, id, name string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/observability/connections/update", nil, map[string]string{"id": id, "name": name}, nil)
}

func (c *Client) RevokeObservabilityConnection(ctx context.Context, id string) error {
	return notFoundAs404(c.do(ctx, http.MethodPost, "/api/v1/observability/connections/revoke", nil, idRequest{ID: id}, nil))
}

// The observability endpoints report a missing connection or service as a 400 "... not found".
func notFoundAs404(err error) error {
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusBadRequest && strings.Contains(apiErr.Body, "not found") {
		apiErr.StatusCode = http.StatusNotFound
	}
	return err
}

type ServiceDataSource struct {
	Type         string  `json:"type"`
	Choice       string  `json:"choice"`
	ConnectionID *string `json:"connection_id"`
}

type ServiceDataSourceInput struct {
	ServiceID    string  `json:"service_id"`
	Type         string  `json:"type"`
	Choice       string  `json:"choice"`
	ConnectionID *string `json:"connection_id"`
}

func (c *Client) GetServiceDataSources(ctx context.Context, serviceID string) ([]ServiceDataSource, error) {
	var out struct {
		Sources []ServiceDataSource `json:"sources"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/service/data-sources", url.Values{"service_id": {serviceID}}, nil, &out)
	return out.Sources, notFoundAs404(err)
}

func (c *Client) SetServiceDataSource(ctx context.Context, in ServiceDataSourceInput) error {
	return c.do(ctx, http.MethodPost, "/api/v1/service/data-sources/update", nil, in, nil)
}
