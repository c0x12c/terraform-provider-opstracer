package client

import (
	"context"
	"net/http"
	"net/url"
)

type Team struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Members []TeamMember `json:"members"`
}

type TeamMember struct {
	// Null for a pending invitation.
	UserID *string `json:"user_id"`
	Status string  `json:"status"`
}

type teamInput struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
}

type teamMembersInput struct {
	TeamID  string   `json:"team_id"`
	UserIDs []string `json:"user_ids,omitempty"`
	UserID  string   `json:"user_id,omitempty"`
}

func (c *Client) CreateTeam(ctx context.Context, name string) (*Team, error) {
	var out Team
	return &out, c.do(ctx, http.MethodPost, "/api/v1/org/teams", nil, teamInput{Name: name}, &out)
}

func (c *Client) GetTeam(ctx context.Context, id string) (*Team, error) {
	var out Team
	return &out, c.do(ctx, http.MethodGet, "/api/v1/org/team", url.Values{"id": {id}}, nil, &out)
}

func (c *Client) RenameTeam(ctx context.Context, id, name string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/org/teams/update", nil, teamInput{ID: id, Name: name}, nil)
}

func (c *Client) DeleteTeam(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/org/teams/delete", nil, idRequest{ID: id}, nil)
}

func (c *Client) AddTeamMembers(ctx context.Context, teamID string, userIDs []string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/org/teams/members/add", nil, teamMembersInput{TeamID: teamID, UserIDs: userIDs}, nil)
}

func (c *Client) RemoveTeamMember(ctx context.Context, teamID, userID string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/org/teams/members/remove", nil, teamMembersInput{TeamID: teamID, UserID: userID}, nil)
}
