package client

import (
	"context"
	"net/http"
	"net/url"
)

type Schedule struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description *string         `json:"description"`
	TimeZone    string          `json:"time_zone"`
	Layers      []ScheduleLayer `json:"layers"`
}

// ScheduleLayer is one rotation. The server omits empty arrays in responses.
type ScheduleLayer struct {
	Name                *string            `json:"name"`
	TimeZone            *string            `json:"time_zone"`
	RotationStartAt     string             `json:"rotation_start_at"`
	MemberPeriodSeconds int64              `json:"member_period_seconds"`
	ParticipantUserIDs  []string           `json:"participant_user_ids"`
	ParticipantGroups   [][]string         `json:"participant_groups"`
	Restrictions        []LayerRestriction `json:"restrictions"`
}

type LayerRestriction struct {
	Days            []int64 `json:"days"`
	StartTimeOfDay  int64   `json:"start_time_of_day"`
	DurationSeconds int64   `json:"duration_seconds"`
}

// ScheduleInput's top-level rotation fields mirror the first layer, as the web app sends them.
type ScheduleInput struct {
	ID                  string          `json:"id,omitempty"`
	Name                string          `json:"name"`
	Description         *string         `json:"description"`
	TimeZone            *string         `json:"time_zone"`
	RotationStartAt     string          `json:"rotation_start_at"`
	MemberPeriodSeconds int64           `json:"member_period_seconds"`
	ParticipantUserIDs  []string        `json:"participant_user_ids"`
	Layers              []ScheduleLayer `json:"layers"`
}

func (c *Client) CreateSchedule(ctx context.Context, in ScheduleInput) (*Schedule, error) {
	var out Schedule
	return &out, c.do(ctx, http.MethodPost, "/api/v1/schedules", nil, in, &out)
}

func (c *Client) GetSchedule(ctx context.Context, id string) (*Schedule, error) {
	var out Schedule
	return &out, c.do(ctx, http.MethodGet, "/api/v1/schedule", url.Values{"id": {id}}, nil, &out)
}

func (c *Client) UpdateSchedule(ctx context.Context, in ScheduleInput) (*Schedule, error) {
	var out Schedule
	return &out, c.do(ctx, http.MethodPost, "/api/v1/schedule/update", nil, in, &out)
}

func (c *Client) DeleteSchedule(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/schedule/delete", nil, idRequest{ID: id}, nil)
}

func (c *Client) ListSchedules(ctx context.Context) ([]Schedule, error) {
	return listAll[Schedule](ctx, c, "/api/v1/schedules/list")
}

// OnCall is who holds a schedule at a point in time. UserIDs is empty when the shift is uncovered.
type OnCall struct {
	ScheduleID string   `json:"schedule_id"`
	At         string   `json:"at"`
	UserIDs    []string `json:"user_ids"`
	Until      *string  `json:"until"`
}

// GetOnCall returns the holders at the given RFC 3339 time, or now when at is empty.
func (c *Client) GetOnCall(ctx context.Context, scheduleID, at string) (*OnCall, error) {
	q := url.Values{"id": {scheduleID}}
	if at != "" {
		q.Set("at", at)
	}
	var out OnCall
	return &out, c.do(ctx, http.MethodGet, "/api/v1/schedule/on-call", q, nil, &out)
}
