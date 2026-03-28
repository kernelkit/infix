// SPDX-License-Identifier: MIT

package restconf

import "context"

// Fetcher is the RESTCONF client interface used by handlers.
// *Client satisfies this interface; testutil.MockFetcher provides a test double.
type Fetcher interface {
	Get(ctx context.Context, path string, target any) error
	GetRaw(ctx context.Context, path string) ([]byte, error)
	Post(ctx context.Context, path string) error
	PostJSON(ctx context.Context, path string, body any) error
}

var _ Fetcher = (*Client)(nil)
