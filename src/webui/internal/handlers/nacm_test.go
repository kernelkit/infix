// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"errors"
	"slices"
	"testing"

	"infix/webui/internal/testutil"
)

func TestWhoamiGroups(t *testing.T) {
	const path = "/operations/infix-system:whoami"

	rc := testutil.NewMockFetcher()
	rc.SetResponse(path, map[string]any{
		"infix-system:output": map[string]any{
			"username": "jane",
			"group":    []string{"guest", "service"},
		},
	})
	got, err := WhoamiGroups(context.Background(), rc)
	if err != nil || !slices.Equal(got, []string{"guest", "service"}) {
		t.Errorf("got %v, %v", got, err)
	}

	rc.SetResponse(path, map[string]any{
		"infix-system:output": map[string]any{"username": "jane"},
	})
	if got, err := WhoamiGroups(context.Background(), rc); err != nil || len(got) != 0 {
		t.Errorf("no groups: got %v, %v", got, err)
	}

	rc.SetError(path, errors.New("access denied"))
	if _, err := WhoamiGroups(context.Background(), rc); err == nil {
		t.Error("denied: no error")
	}
}
