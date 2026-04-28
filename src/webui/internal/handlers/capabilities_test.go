// SPDX-License-Identifier: MIT

package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/kernelkit/webui/internal/testutil"
)

func yangLibraryResponse(modules ...map[string]interface{}) map[string]interface{} {
	mods := make([]interface{}, len(modules))
	for i, m := range modules {
		mods[i] = m
	}
	return map[string]interface{}{
		"ietf-yang-library:yang-library": map[string]interface{}{
			"module-set": []interface{}{
				map[string]interface{}{"module": mods},
			},
		},
	}
}

func module(name string, features ...string) map[string]interface{} {
	m := map[string]interface{}{"name": name}
	if len(features) > 0 {
		m["feature"] = features
	}
	return m
}

func TestDetectCapabilities_NoModules(t *testing.T) {
	mock := testutil.NewMockFetcher()
	mock.SetResponse("/data/ietf-yang-library:yang-library", yangLibraryResponse())

	caps := DetectCapabilities(context.Background(), mock)
	if caps.Has("wifi") || caps.Has("containers") {
		t.Errorf("expected no features, got wifi=%v containers=%v",
			caps.Has("wifi"), caps.Has("containers"))
	}
}

func TestDetectCapabilities_YangLibraryError(t *testing.T) {
	mock := testutil.NewMockFetcher()
	mock.SetError("/data/ietf-yang-library:yang-library", errors.New("unreachable"))

	caps := DetectCapabilities(context.Background(), mock)
	if caps.Has("wifi") || caps.Has("containers") {
		t.Errorf("expected no features on error, got wifi=%v containers=%v",
			caps.Has("wifi"), caps.Has("containers"))
	}
}

func TestDetectCapabilities_ContainersModule(t *testing.T) {
	mock := testutil.NewMockFetcher()
	mock.SetResponse("/data/ietf-yang-library:yang-library",
		yangLibraryResponse(
			module("ietf-interfaces"),
			module("infix-interfaces", "vlan-filtering", "containers"),
		))

	caps := DetectCapabilities(context.Background(), mock)
	if !caps.Has("containers") {
		t.Error("expected containers=true")
	}
	if caps.Has("wifi") {
		t.Error("expected wifi=false")
	}
}

func TestDetectCapabilities_WiFiModule(t *testing.T) {
	mock := testutil.NewMockFetcher()
	mock.SetResponse("/data/ietf-yang-library:yang-library",
		yangLibraryResponse(
			module("ietf-interfaces"),
			module("infix-interfaces", "vlan-filtering", "wifi"),
		))

	caps := DetectCapabilities(context.Background(), mock)
	if !caps.Has("wifi") {
		t.Error("expected wifi=true")
	}
	if caps.Has("containers") {
		t.Error("expected containers=false")
	}
}

func TestDetectCapabilities_BothModules(t *testing.T) {
	mock := testutil.NewMockFetcher()
	mock.SetResponse("/data/ietf-yang-library:yang-library",
		yangLibraryResponse(
			module("ietf-interfaces"),
			module("infix-interfaces", "vlan-filtering", "containers", "wifi"),
		))

	caps := DetectCapabilities(context.Background(), mock)
	if !caps.Has("wifi") || !caps.Has("containers") {
		t.Errorf("expected both features, got wifi=%v containers=%v",
			caps.Has("wifi"), caps.Has("containers"))
	}
}

func TestCapabilitiesFromContext_NilReturnsEmpty(t *testing.T) {
	caps := CapabilitiesFromContext(context.Background())
	if caps == nil {
		t.Fatal("expected non-nil Capabilities")
	}
	if caps.Has("wifi") || caps.Has("containers") {
		t.Error("expected no features for empty context")
	}
}

func TestCapabilitiesFromContext_RoundTrip(t *testing.T) {
	orig := NewCapabilities(map[string]bool{"wifi": true, "containers": true})
	ctx := ContextWithCapabilities(context.Background(), orig)
	got := CapabilitiesFromContext(ctx)
	if !got.Has("wifi") || !got.Has("containers") {
		t.Errorf("expected wifi=true containers=true, got wifi=%v containers=%v",
			got.Has("wifi"), got.Has("containers"))
	}
}
