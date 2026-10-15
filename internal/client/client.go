package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	baseURL   string
	token     string
	userAgent string
	http      *http.Client
}

func New(baseURL, token, userAgent string) *Client {
	return &Client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		token:     token,
		userAgent: userAgent,
		http:      &http.Client{Timeout: 30 * time.Second},
	}
}

type APIError struct {
	Method     string
	Path       string
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s %s: HTTP %d: %s", e.Method, e.Path, e.StatusCode, e.Body)
}

func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

const maxAttempts = 3

func (c *Client) do(ctx context.Context, method, path string, query url.Values, in, out any) error {
	var payload []byte
	if in != nil {
		var err error
		if payload, err = json.Marshal(in); err != nil {
			return err
		}
	}

	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	for attempt := 1; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", c.userAgent)
		if in != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.http.Do(req)
		if err != nil {
			return err
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}

		if retryable(resp.StatusCode) && attempt < maxAttempts {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
			continue
		}
		if resp.StatusCode >= 300 {
			return &APIError{Method: method, Path: path, StatusCode: resp.StatusCode, Body: string(body)}
		}
		if out == nil || len(body) == 0 {
			return nil
		}
		// Every response is wrapped as {"data": ...}; the OpenAPI spec does not show this.
		var env struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(body, &env); err != nil {
			return err
		}
		return json.Unmarshal(env.Data, out)
	}
}

func retryable(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusBadGateway ||
		status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

type idRequest struct {
	ID string `json:"id"`
}

// IsGone reports a 404 or 410, which some endpoints use for objects that no longer exist.
func IsGone(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusNotFound || apiErr.StatusCode == http.StatusGone)
}
