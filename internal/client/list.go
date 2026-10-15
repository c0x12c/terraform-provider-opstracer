package client

import (
	"context"
	"net/http"
)

const pageSize = 50

type listRequest struct {
	Limit  int   `json:"limit"`
	Offset int64 `json:"offset"`
}

type page[T any] struct {
	Total int64 `json:"total"`
	Data  []T   `json:"data"`
}

// listAll walks the offset-paginated POST .../list endpoints.
func listAll[T any](ctx context.Context, c *Client, path string) ([]T, error) {
	return listAllWith[T](ctx, c, path, func(r listRequest) any { return r })
}

// listAllWith is listAll for endpoints that need extra filter fields next to limit and offset.
func listAllWith[T any](ctx context.Context, c *Client, path string, body func(listRequest) any) ([]T, error) {
	var all []T
	for {
		var p page[T]
		if err := c.do(ctx, http.MethodPost, path, nil, body(listRequest{Limit: pageSize, Offset: int64(len(all))}), &p); err != nil {
			return nil, err
		}
		all = append(all, p.Data...)
		if len(p.Data) == 0 || int64(len(all)) >= p.Total {
			return all, nil
		}
	}
}
