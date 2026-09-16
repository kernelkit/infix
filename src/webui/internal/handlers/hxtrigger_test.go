// SPDX-License-Identifier: MIT

package handlers

import (
	"net/http/httptest"
	"testing"
)

func TestHxTriggerIsASCII(t *testing.T) {
	w := httptest.NewRecorder()
	hxTrigger(w, "cfgError", `channel must be 1–196, not "x" 😀`)
	got := w.Header().Get("HX-Trigger")
	want := `{"cfgError":"channel must be 1\u2013196, not \"x\" \ud83d\ude00"}`
	if got != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}
	for i := 0; i < len(got); i++ {
		if got[i] >= 0x80 {
			t.Fatalf("non-ASCII byte at %d in %q", i, got)
		}
	}
}
