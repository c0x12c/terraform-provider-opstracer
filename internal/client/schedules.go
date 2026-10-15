package client

import (
	"context"
	"net/http"
	"net/url"
	"time"
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

type ScheduleAnnouncement struct {
	ScheduleID       string  `json:"schedule_id"`
	Enabled          bool    `json:"enabled"`
	SlackChannelID   *string `json:"slack_channel_id"`
	SlackChannelName *string `json:"slack_channel_name"`
}

func (c *Client) GetScheduleAnnouncement(ctx context.Context, scheduleID string) (*ScheduleAnnouncement, error) {
	var out ScheduleAnnouncement
	return &out, c.do(ctx, http.MethodGet, "/api/v1/schedule/announcement", url.Values{"schedule_id": {scheduleID}}, nil, &out)
}

// UpdateScheduleAnnouncement sets the Slack channel for shift-change posts; nil channel fields turn it off.
func (c *Client) UpdateScheduleAnnouncement(ctx context.Context, in ScheduleAnnouncement) (*ScheduleAnnouncement, error) {
	var out ScheduleAnnouncement
	body := struct {
		ScheduleID       string  `json:"schedule_id"`
		SlackChannelID   *string `json:"slack_channel_id"`
		SlackChannelName *string `json:"slack_channel_name"`
	}{in.ScheduleID, in.SlackChannelID, in.SlackChannelName}
	return &out, c.do(ctx, http.MethodPost, "/api/v1/schedule/announcement/update", nil, body, &out)
}

type ScheduleOverride struct {
	ID             string  `json:"id,omitempty"`
	ScheduleID     string  `json:"schedule_id"`
	UserID         string  `json:"user_id"`
	StartsAt       string  `json:"starts_at"`
	EndsAt         string  `json:"ends_at"`
	ReplacesUserID *string `json:"replaces_user_id"`
	CreatedBy      string  `json:"created_by,omitempty"`
}

// MaxOverrideListWindow is the widest from/to range the list endpoint accepts.
const MaxOverrideListWindow = 90 * 24 * time.Hour

func (c *Client) CreateScheduleOverride(ctx context.Context, in ScheduleOverride) (*ScheduleOverride, error) {
	var out ScheduleOverride
	return &out, c.do(ctx, http.MethodPost, "/api/v1/schedule/overrides", nil, in, &out)
}

// ListScheduleOverrides returns overrides overlapping [from, to). The result is not paginated.
func (c *Client) ListScheduleOverrides(ctx context.Context, scheduleID string, from, to time.Time) ([]ScheduleOverride, error) {
	var out []ScheduleOverride
	in := struct {
		ScheduleID string `json:"schedule_id"`
		From       string `json:"from"`
		To         string `json:"to"`
	}{scheduleID, from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339)}
	return out, c.do(ctx, http.MethodPost, "/api/v1/schedule/overrides/list", nil, in, &out)
}

func (c *Client) DeleteScheduleOverride(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/schedule/override/delete", nil, idRequest{ID: id}, nil)
}
