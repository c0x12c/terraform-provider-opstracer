package client

import (
	"context"
	"net/http"
	"net/url"
)

type NotificationOrder struct {
	SchemaVersion int                `json:"schema_version"`
	Enabled       bool               `json:"enabled"`
	Tiers         []NotificationTier `json:"tiers"`
}

type NotificationTier struct {
	Channels     []string `json:"channels"`
	DelaySeconds int64    `json:"delay_seconds"`
}

func (c *Client) GetNotificationOrder(ctx context.Context) (*NotificationOrder, error) {
	var out NotificationOrder
	return &out, c.do(ctx, http.MethodGet, "/api/v1/org/notification-order", nil, nil, &out)
}

func (c *Client) SetNotificationOrder(ctx context.Context, in NotificationOrder) (*NotificationOrder, error) {
	in.SchemaVersion = 1
	var out NotificationOrder
	return &out, c.do(ctx, http.MethodPost, "/api/v1/org/notification-order", nil, in, &out)
}

type MemberTitle struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Scope string `json:"scope"`
}

type titleInput struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
}

func (c *Client) ListMemberTitles(ctx context.Context) ([]MemberTitle, error) {
	var out []MemberTitle
	return out, c.do(ctx, http.MethodGet, "/api/v1/org/titles", nil, nil, &out)
}

func (c *Client) CreateMemberTitle(ctx context.Context, name string) (*MemberTitle, error) {
	var out MemberTitle
	return &out, c.do(ctx, http.MethodPost, "/api/v1/org/titles", nil, titleInput{Name: name}, &out)
}

func (c *Client) UpdateMemberTitle(ctx context.Context, id, name string) (*MemberTitle, error) {
	var out MemberTitle
	return &out, c.do(ctx, http.MethodPost, "/api/v1/org/titles/update", nil, titleInput{ID: id, Name: name}, &out)
}

func (c *Client) DeleteMemberTitle(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/org/titles/delete", nil, idRequest{ID: id}, nil)
}

type Member struct {
	UserID      string       `json:"user_id"`
	Email       string       `json:"email"`
	DisplayName *string      `json:"display_name"`
	Role        string       `json:"role"`
	Title       *MemberTitle `json:"title"`
}

func (c *Client) ListMembers(ctx context.Context) ([]Member, error) {
	return listAll[Member](ctx, c, "/api/v1/org/members/list")
}

func (c *Client) UpdateMemberRole(ctx context.Context, userID, role string) (*Member, error) {
	var out Member
	in := struct {
		Role string `json:"role"`
	}{role}
	return &out, c.do(ctx, http.MethodPost, "/api/v1/org/members/update-role", url.Values{"user_id": {userID}}, in, &out)
}

// AssignMemberTitle assigns a title; a nil titleID clears it.
func (c *Client) AssignMemberTitle(ctx context.Context, userID string, titleID *string) (*Member, error) {
	var out Member
	in := struct {
		TitleID *string `json:"title_id"`
	}{titleID}
	return &out, c.do(ctx, http.MethodPost, "/api/v1/org/members/update-title", url.Values{"user_id": {userID}}, in, &out)
}
