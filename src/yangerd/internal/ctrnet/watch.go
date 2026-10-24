package ctrnet

import (
	"log/slog"
	"sync"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

// Watcher follows link and address events in the container network
// namespaces, which the host's netlink subscriptions never see, and
// calls kick on each one so the interfaces are collected again.
type Watcher struct {
	kick func()
	log  *slog.Logger

	// subscribe is overridable in tests.
	subscribe func(name string) (chan struct{}, error)

	mu   sync.Mutex
	subs map[string]chan struct{} // netns name → closes its subscription
}

// NewWatcher returns a Watcher that calls kick on every event.
func NewWatcher(kick func(), log *slog.Logger) *Watcher {
	if log == nil {
		log = slog.Default()
	}
	w := &Watcher{kick: kick, log: log, subs: make(map[string]chan struct{})}
	w.subscribe = w.netlinkSubscribe
	return w
}

// Follow subscribes to the namespaces in names that it does not follow
// yet and drops the rest.  A namespace that fails is tried again on the
// next call.
func (w *Watcher) Follow(names []string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	for name, done := range w.subs {
		if !contains(names, name) {
			close(done)
			delete(w.subs, name)
		}
	}
	for _, name := range names {
		if _, ok := w.subs[name]; ok {
			continue
		}
		done, err := w.subscribe(name)
		if err != nil {
			w.log.Debug("container netns: subscribe failed", "netns", name, "err", err)
			continue
		}
		w.subs[name] = done
	}
}

// Close drops every subscription.
func (w *Watcher) Close() {
	w.Follow(nil)
}

func (w *Watcher) netlinkSubscribe(name string) (chan struct{}, error) {
	ns, err := netns.GetFromName(name)
	if err != nil {
		return nil, err
	}
	defer ns.Close()

	done := make(chan struct{})
	links := make(chan netlink.LinkUpdate, 16)
	addrs := make(chan netlink.AddrUpdate, 16)
	failed := func(err error) {
		w.log.Debug("container netns: subscription error", "netns", name, "err", err)
	}

	err = netlink.LinkSubscribeWithOptions(links, done, netlink.LinkSubscribeOptions{
		Namespace:     &ns,
		ErrorCallback: failed,
	})
	if err != nil {
		close(done)
		return nil, err
	}
	err = netlink.AddrSubscribeWithOptions(addrs, done, netlink.AddrSubscribeOptions{
		Namespace:     &ns,
		ErrorCallback: failed,
	})
	if err != nil {
		close(done)
		return nil, err
	}

	go w.drain(name, done, links, addrs)
	return done, nil
}

// drain kicks on every event until both subscriptions end.  One that
// ends on its own, the namespace gone or a receive error, is forgotten
// so the next Follow subscribes again.
func (w *Watcher) drain(name string, done chan struct{}, links <-chan netlink.LinkUpdate, addrs <-chan netlink.AddrUpdate) {
	for links != nil || addrs != nil {
		select {
		case _, ok := <-links:
			if !ok {
				links = nil
				continue
			}
		case _, ok := <-addrs:
			if !ok {
				addrs = nil
				continue
			}
		}
		w.kick()
	}

	w.mu.Lock()
	if w.subs[name] == done {
		delete(w.subs, name)
		close(done)
		w.mu.Unlock()
		w.kick()
		return
	}
	w.mu.Unlock()
}
