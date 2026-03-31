package tree

import (
	"encoding/json"
	"sync"
	"testing"
)

func TestSetGet(t *testing.T) {
	tr := New()
	tr.Set("ietf-system:system", json.RawMessage(`{"hostname":"r1"}`))

	got := tr.Get("ietf-system:system")
	if string(got) != `{"hostname":"r1"}` {
		t.Fatalf("unexpected: %s", got)
	}
}

func TestGetMissing(t *testing.T) {
	tr := New()
	if got := tr.Get("nonexistent"); got != nil {
		t.Fatalf("expected nil, got: %s", got)
	}
}

func TestSetOverwrite(t *testing.T) {
	tr := New()
	tr.Set("key", json.RawMessage(`"v1"`))
	tr.Set("key", json.RawMessage(`"v2"`))

	if got := tr.Get("key"); string(got) != `"v2"` {
		t.Fatalf("expected v2, got: %s", got)
	}
}

func TestGetMulti(t *testing.T) {
	tr := New()
	tr.Set("a", json.RawMessage(`1`))
	tr.Set("b", json.RawMessage(`2`))
	tr.Set("c", json.RawMessage(`3`))

	results := tr.GetMulti([]string{"a", "c"})
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if string(results[0]) != "1" || string(results[1]) != "3" {
		t.Fatalf("unexpected results: %s, %s", results[0], results[1])
	}
}

func TestGetMultiMissing(t *testing.T) {
	tr := New()
	tr.Set("a", json.RawMessage(`1`))

	results := tr.GetMulti([]string{"a", "missing"})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestKeys(t *testing.T) {
	tr := New()
	tr.Set("x", json.RawMessage(`1`))
	tr.Set("y", json.RawMessage(`2`))

	keys := tr.Keys()
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
}

func TestInfo(t *testing.T) {
	tr := New()
	tr.Set("k", json.RawMessage(`{"data":true}`))

	info, ok := tr.Info("k")
	if !ok {
		t.Fatal("expected ok")
	}
	if info.SizeBytes != len(`{"data":true}`) {
		t.Fatalf("expected size %d, got %d", len(`{"data":true}`), info.SizeBytes)
	}
	if info.LastUpdated.IsZero() {
		t.Fatal("expected non-zero LastUpdated")
	}
}

func TestInfoMissing(t *testing.T) {
	tr := New()
	_, ok := tr.Info("missing")
	if ok {
		t.Fatal("expected !ok for missing key")
	}
}

func TestConcurrentSetGet(t *testing.T) {
	tr := New()
	var wg sync.WaitGroup
	const N = 100

	for i := 0; i < N; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			tr.Set("shared", json.RawMessage(`{"i":`+string(rune('0'+i%10))+`}`))
		}(i)
		go func() {
			defer wg.Done()
			tr.Get("shared")
		}()
	}
	wg.Wait()

	if got := tr.Get("shared"); got == nil {
		t.Fatal("expected non-nil after concurrent writes")
	}
}

func TestMerge(t *testing.T) {
	tests := []struct {
		name     string
		existing string
		partial  string
		want     map[string]json.RawMessage
	}{
		{
			name:     "merge into existing preserves old fields",
			existing: `{"a":"1","b":"2"}`,
			partial:  `{"c":"3"}`,
			want: map[string]json.RawMessage{
				"a": json.RawMessage(`"1"`),
				"b": json.RawMessage(`"2"`),
				"c": json.RawMessage(`"3"`),
			},
		},
		{
			name:     "merge overwrites overlapping field",
			existing: `{"a":"old","b":"keep"}`,
			partial:  `{"a":"new"}`,
			want: map[string]json.RawMessage{
				"a": json.RawMessage(`"new"`),
				"b": json.RawMessage(`"keep"`),
			},
		},
		{
			name:     "merge with complex nested values",
			existing: `{"protocols":{"ospf":true}}`,
			partial:  `{"ribs":{"rib":[{"name":"ipv4"}]}}`,
			want: map[string]json.RawMessage{
				"protocols": json.RawMessage(`{"ospf":true}`),
				"ribs":      json.RawMessage(`{"rib":[{"name":"ipv4"}]}`),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := New()
			tr.Set("k", json.RawMessage(tc.existing))
			tr.Merge("k", json.RawMessage(tc.partial))

			var got map[string]json.RawMessage
			if err := json.Unmarshal(tr.Get("k"), &got); err != nil {
				t.Fatalf("unmarshal result: %v", err)
			}
			for field, wantVal := range tc.want {
				gotVal, ok := got[field]
				if !ok {
					t.Fatalf("missing field %q", field)
				}
				if string(gotVal) != string(wantVal) {
					t.Fatalf("field %q: got %s, want %s", field, gotVal, wantVal)
				}
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %d fields, want %d", len(got), len(tc.want))
			}
		})
	}
}

func TestMergeIntoEmpty(t *testing.T) {
	tr := New()
	tr.Merge("new-key", json.RawMessage(`{"x":1}`))
	got := tr.Get("new-key")
	if string(got) != `{"x":1}` {
		t.Fatalf("expected {\"x\":1}, got %s", got)
	}
}

func TestMergeNonObjectFallback(t *testing.T) {
	tr := New()
	tr.Set("k", json.RawMessage(`{"a":"1"}`))
	tr.Merge("k", json.RawMessage(`"plain string"`))
	got := tr.Get("k")
	if string(got) != `"plain string"` {
		t.Fatalf("expected plain string fallback, got %s", got)
	}
}

func TestDelete(t *testing.T) {
	tr := New()
	tr.Set("k", json.RawMessage(`{"data":true}`))
	tr.Delete("k")
	if got := tr.Get("k"); got != nil {
		t.Fatalf("expected nil after delete, got %s", got)
	}
}

func TestDeleteMissing(t *testing.T) {
	tr := New()
	tr.Delete("nonexistent")
}

func TestGetWithProvider(t *testing.T) {
	tr := New()
	tr.Set("k", json.RawMessage(`{"cached":"yes","kept":"ok"}`))
	tr.RegisterProvider("k", func() json.RawMessage {
		return json.RawMessage(`{"live":"data","cached":"overridden"}`)
	})

	got := tr.Get("k")
	var m map[string]string
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["live"] != "data" {
		t.Fatalf("expected live=data, got %q", m["live"])
	}
	if m["cached"] != "overridden" {
		t.Fatalf("expected provider to override cached field, got %q", m["cached"])
	}
	if m["kept"] != "ok" {
		t.Fatalf("expected kept=ok preserved, got %q", m["kept"])
	}
}

func TestGetWithProviderDoesNotMutateCache(t *testing.T) {
	tr := New()
	tr.Set("k", json.RawMessage(`{"a":"1"}`))
	tr.RegisterProvider("k", func() json.RawMessage {
		return json.RawMessage(`{"b":"2"}`)
	})

	tr.Get("k")

	// Read the raw cached entry — remove the provider to bypass merge.
	tr.RegisterProvider("k", nil)
	raw := tr.Get("k")
	if string(raw) != `{"a":"1"}` {
		t.Fatalf("cache was mutated: %s", raw)
	}
}

func TestGetMultiWithProvider(t *testing.T) {
	tr := New()
	tr.Set("a", json.RawMessage(`{"x":"1"}`))
	tr.Set("b", json.RawMessage(`{"y":"2"}`))
	tr.RegisterProvider("a", func() json.RawMessage {
		return json.RawMessage(`{"live":"yes"}`)
	})

	results := tr.GetMulti([]string{"a", "b"})
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	var m map[string]string
	json.Unmarshal(results[0], &m)
	if m["live"] != "yes" || m["x"] != "1" {
		t.Fatalf("provider not applied to first result: %s", results[0])
	}

	// b has no provider — should return as-is
	if string(results[1]) != `{"y":"2"}` {
		t.Fatalf("unexpected second result: %s", results[1])
	}
}

func TestGetWithProviderEmptyOverlay(t *testing.T) {
	tr := New()
	tr.Set("k", json.RawMessage(`{"a":"1"}`))
	tr.RegisterProvider("k", func() json.RawMessage {
		return nil
	})

	got := tr.Get("k")
	if string(got) != `{"a":"1"}` {
		t.Fatalf("nil overlay should return base, got %s", got)
	}
}

func TestGetWithProviderNoBaseData(t *testing.T) {
	tr := New()
	tr.Set("k", json.RawMessage(nil))
	tr.RegisterProvider("k", func() json.RawMessage {
		return json.RawMessage(`{"live":"yes"}`)
	})

	got := tr.Get("k")
	if string(got) != `{"live":"yes"}` {
		t.Fatalf("expected overlay to win with empty base, got %s", got)
	}
}

func TestConcurrentMerge(t *testing.T) {
	tr := New()
	tr.Set("shared", json.RawMessage(`{}`))

	var wg sync.WaitGroup
	const N = 50
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tr.Merge("shared", json.RawMessage(`{"f`+string(rune('a'+i%26))+`":true}`))
		}(i)
	}
	wg.Wait()

	got := tr.Get("shared")
	if got == nil {
		t.Fatal("expected non-nil after concurrent merges")
	}
}
