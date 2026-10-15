package client

import (
	"context"
	"net/http"
)

// Mute silences a whole service or integration for a time window. Ended and expired mutes drop out of the list.
type Mute struct {
	ID        string  `json:"id"`
	ScopeType string  `json:"scope_type"`
	ScopeID   string  `json:"scope_id"`
	ScopeName *string `json:"scope_name"`
	ServiceID *string `json:"service_id"`
	StartsAt  string  `json:"starts_at"`
	EndsAt    string  `json:"ends_at"`
	Reason    *string `json:"reason"`
	Active    bool    `json:"active"`
}

type MuteInput struct {
	ScopeType string  `json:"scope_type"`
	ScopeID   string  `json:"scope_id"`
	StartsAt  *string `json:"starts_at"`
	EndsAt    string  `json:"ends_at"`
	Reason    *string `json:"reason"`
}

// ListMutes returns the scheduled and running mutes.
func (c *Client) ListMutes(ctx context.Context) ([]Mute, error) {
	return listAll[Mute](ctx, c, "/api/v1/mutes/list")
}

func (c *Client) CreateMute(ctx context.Context, in MuteInput) (*Mute, error) {
	var out Mute
	return &out, c.do(ctx, http.MethodPost, "/api/v1/mutes", nil, in, &out)
}

// EndMute stops a running mute now; the server rejects it for a mute that has not started.
func (c *Client) EndMute(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/mutes/end", nil, idRequest{ID: id}, nil)
}

// DeleteMute removes a scheduled mute; the server rejects it once the mute has started.
func (c *Client) DeleteMute(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/mutes/delete", nil, idRequest{ID: id}, nil)
}
