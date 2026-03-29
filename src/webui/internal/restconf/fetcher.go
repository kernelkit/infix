// SPDX-License-Identifier: MIT

package restconf

import (
	"context"
	"encoding/json"
)

// Fetcher is the RESTCONF client interface used by handlers.
// *Client satisfies this interface; testutil.MockFetcher provides a test double.
type Fetcher interface {
	Get(ctx context.Context, path string, target any) error
	GetRaw(ctx context.Context, path string) ([]byte, error)
	Post(ctx context.Context, path string) error
	PostJSON(ctx context.Context, path string, body any) error

	Put(ctx context.Context, path string, body any) error
	Patch(ctx context.Context, path string, body any) error
	Delete(ctx context.Context, path string) error

	GetDatastore(ctx context.Context, datastore string) (json.RawMessage, error)
	PutDatastore(ctx context.Context, datastore string, body json.RawMessage) error
	CopyDatastore(ctx context.Context, src, dst string) error
}

var _ Fetcher = (*Client)(nil)
