// SPDX-License-Identifier: MIT

package restconf

import (
	"context"
	"net/http"
	"strconv"
)

// YangPatch is an RFC 8072 YANG Patch: a list of edits sent to one target
// resource in a single request.  rousette applies them in one sysrepo edit
// batch, so the datastore takes all of the edits or none of them.
type YangPatch struct {
	Target string
	Edits  []YangEdit
}

// YangEdit is one edit of a YangPatch.  Target is an api-path relative to
// the patch target; Value is the node wrapped in its qualified name, the
// same shape a PUT body takes, and nil for remove.
type YangEdit struct {
	ID     string `json:"edit-id"`
	Op     string `json:"operation"`
	Target string `json:"target"`
	Value  any    `json:"value,omitempty"`
}

// NewYangPatch starts an empty patch against the resource at target.
func NewYangPatch(target string) *YangPatch {
	return &YangPatch{Target: target}
}

func (p *YangPatch) add(op, target string, value any) *YangPatch {
	p.Edits = append(p.Edits, YangEdit{
		ID:     "e" + strconv.Itoa(len(p.Edits)+1),
		Op:     op,
		Target: target,
		Value:  value,
	})
	return p
}

// Merge merges value into target, creating it when absent.
func (p *YangPatch) Merge(target string, value any) *YangPatch {
	return p.add("merge", target, value)
}

// Replace replaces target with value, creating it when absent.
func (p *YangPatch) Replace(target string, value any) *YangPatch {
	return p.add("replace", target, value)
}

// Remove deletes target.  Unlike delete it is not an error when the
// target is already absent.
func (p *YangPatch) Remove(target string) *YangPatch {
	return p.add("remove", target, nil)
}

// Body returns the request body in the ietf-yang-patch JSON encoding.
func (p *YangPatch) Body() map[string]any {
	return map[string]any{
		"ietf-yang-patch:yang-patch": map[string]any{
			"patch-id": "webui",
			"edit":     p.Edits,
		},
	}
}

// YangPatch sends p as a single PATCH request.  A patch with no edits is
// a no-op.
func (c *Client) YangPatch(ctx context.Context, p *YangPatch) error {
	if len(p.Edits) == 0 {
		return nil
	}
	return c.write(ctx, http.MethodPatch, p.Target, "application/yang-patch+json", p.Body())
}
