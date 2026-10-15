package client

import (
	"context"
	"net/http"
	"net/url"
)

type EscalationPolicy struct {
	ID             string        `json:"id"`
	Name           string        `json:"name"`
	Description    *string       `json:"description"`
	Levels         []PolicyLevel `json:"levels"`
	RepeatCount    int32         `json:"repeat_count"`
	AckTimeoutS    int64         `json:"ack_timeout_s"`
	RepageAfterAck bool          `json:"repage_after_ack"`
	IsDefault      bool          `json:"is_default"`
}

type PolicyLevel struct {
	Targets  []PolicyTarget `json:"targets"`
	TimeoutS *int64         `json:"timeout_s"`
}

type PolicyTarget struct {
	Type       string  `json:"type"`
	UserID     *string `json:"user_id"`
	ScheduleID *string `json:"schedule_id"`
}

type EscalationPolicyInput struct {
	ID             string        `json:"id,omitempty"`
	Name           string        `json:"name"`
	Description    *string       `json:"description"`
	Levels         []PolicyLevel `json:"levels"`
	RepeatCount    *int32        `json:"repeat_count"`
	AckTimeoutS    *int64        `json:"ack_timeout_s"`
	RepageAfterAck *bool         `json:"repage_after_ack"`
}

func (c *Client) CreateEscalationPolicy(ctx context.Context, in EscalationPolicyInput) (*EscalationPolicy, error) {
	var out EscalationPolicy
	return &out, c.do(ctx, http.MethodPost, "/api/v1/escalation-policies", nil, in, &out)
}

func (c *Client) GetEscalationPolicy(ctx context.Context, id string) (*EscalationPolicy, error) {
	var out EscalationPolicy
	return &out, c.do(ctx, http.MethodGet, "/api/v1/escalation-policy", url.Values{"id": {id}}, nil, &out)
}

func (c *Client) UpdateEscalationPolicy(ctx context.Context, in EscalationPolicyInput) (*EscalationPolicy, error) {
	var out EscalationPolicy
	return &out, c.do(ctx, http.MethodPost, "/api/v1/escalation-policies/update", nil, in, &out)
}

func (c *Client) DeleteEscalationPolicy(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/escalation-policies/delete", nil, idRequest{ID: id}, nil)
}
