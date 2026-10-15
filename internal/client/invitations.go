package client

import (
	"context"
	"net/http"
	"net/url"
)

type Invitation struct {
	ID        string  `json:"id"`
	Email     string  `json:"email"`
	Role      string  `json:"role"`
	Status    string  `json:"status"`
	InviteURL *string `json:"invite_url"`
	Teams     []struct {
		ID string `json:"id"`
	} `json:"teams"`
}

type InvitationInput struct {
	Email     string   `json:"email"`
	Role      string   `json:"role"`
	TeamIDs   []string `json:"team_ids"`
	SendEmail bool     `json:"send_email"`
}

// CreateInvitation also updates role and teams of a pending invitation for the same email, keeping its ID.
func (c *Client) CreateInvitation(ctx context.Context, in InvitationInput) (*Invitation, error) {
	if in.TeamIDs == nil {
		in.TeamIDs = []string{}
	}
	var out Invitation
	return &out, c.do(ctx, http.MethodPost, "/api/v1/org/invitations", nil, in, &out)
}

func (c *Client) ListPendingInvitations(ctx context.Context) ([]Invitation, error) {
	return listAllWith[Invitation](ctx, c, "/api/v1/org/invitations/list", func(r listRequest) any {
		return struct {
			listRequest
			Status string `json:"status"`
		}{r, "pending"}
	})
}

func (c *Client) ResendInvitation(ctx context.Context, id string, sendEmail bool) (*Invitation, error) {
	var out Invitation
	in := struct {
		SendEmail bool `json:"send_email"`
	}{sendEmail}
	return &out, c.do(ctx, http.MethodPost, "/api/v1/org/invitations/resend", url.Values{"id": {id}}, in, &out)
}

// RevokeInvitation answers 410 Gone when the invitation is already revoked, accepted or unknown.
func (c *Client) RevokeInvitation(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/org/invitations/revoke", url.Values{"id": {id}}, nil, nil)
}
