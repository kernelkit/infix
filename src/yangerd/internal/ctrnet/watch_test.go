package ctrnet

import (
	"errors"
	"reflect"
	"sort"
	"testing"
)

func TestWatcherFollow(t *testing.T) {
	w := NewWatcher(func() {}, nil)
	var subscribed []string
	fail := map[string]bool{"netns-bad": true}
	w.subscribe = func(name string) (chan struct{}, error) {
		subscribed = append(subscribed, name)
		if fail[name] {
			return nil, errors.New("no such netns")
		}
		return make(chan struct{}), nil
	}
	following := func() []string {
		var names []string
		for name := range w.subs {
			names = append(names, name)
		}
		sort.Strings(names)
		return names
	}

	w.Follow([]string{"netns-1", "netns-2", "netns-bad"})
	if got := following(); !reflect.DeepEqual(got, []string{"netns-1", "netns-2"}) {
		t.Fatalf("following %v", got)
	}
	gone := w.subs["netns-1"]

	subscribed = nil
	w.Follow([]string{"netns-2", "netns-bad", "netns-3"})
	if got := following(); !reflect.DeepEqual(got, []string{"netns-2", "netns-3"}) {
		t.Fatalf("following %v", got)
	}
	if !reflect.DeepEqual(subscribed, []string{"netns-bad", "netns-3"}) {
		t.Fatalf("subscribed %v, want only the new and the failed one", subscribed)
	}
	select {
	case <-gone:
	default:
		t.Fatal("subscription of a dropped netns not closed")
	}

	w.Close()
	if got := following(); len(got) != 0 {
		t.Fatalf("following %v after Close", got)
	}
}
