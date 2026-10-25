// Package mmmonitor keeps the modems, bearers and SIM cards ModemManager
// manages, the same properties mmcli shows, updated from its D-Bus
// signals.
package mmmonitor

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"sync"

	"github.com/godbus/dbus/v5"

	"github.com/kernelkit/infix/src/yangerd/internal/backoff"
)

const (
	dbusName      = "org.freedesktop.DBus"
	objectManager = dbusName + ".ObjectManager"
	properties    = dbusName + ".Properties"
)

// source is ModemManager on the bus, an interface for the tests.
type source interface {
	owner(ctx context.Context) (string, error)
	objects(ctx context.Context) (map[dbus.ObjectPath]map[string]props, error)
	getAll(ctx context.Context, path, iface string) (props, error)
}

// Monitor holds ModemManager's objects as its signals leave them.
type Monitor struct {
	log   *slog.Logger
	owner string // unique bus name of ModemManager, the only sender trusted

	mu   sync.Mutex
	objs map[dbus.ObjectPath]map[string]props // modems, by interface
	refs map[string]props                     // their bearers and SIMs
}

// New creates a Monitor, it knows no modems until Run reads them.
func New(log *slog.Logger) *Monitor {
	return &Monitor{log: log}
}

// Modems is every modem ModemManager manages, as last signalled.
func (m *Monitor) Modems() []Modem {
	m.mu.Lock()
	defer m.mu.Unlock()
	return modems(m.objs, m.refs)
}

// Run follows ModemManager until ctx ends, reconnecting with backoff.
func (m *Monitor) Run(ctx context.Context) error {
	return backoff.Retry(ctx, m.log, "modem monitor", m.session)
}

func (m *Monitor) session(ctx context.Context) error {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return fmt.Errorf("connect system bus: %w", err)
	}
	defer conn.Close()

	for _, opts := range [][]dbus.MatchOption{
		{dbus.WithMatchSender(dbusName), dbus.WithMatchInterface(dbusName),
			dbus.WithMatchMember("NameOwnerChanged"),
			dbus.WithMatchArg(0, service)},
		{dbus.WithMatchSender(service), dbus.WithMatchInterface(objectManager)},
		{dbus.WithMatchSender(service), dbus.WithMatchInterface(properties),
			dbus.WithMatchMember("PropertiesChanged"), dbus.WithMatchPathNamespace(root)},
	} {
		if err := conn.AddMatchSignal(opts...); err != nil {
			return fmt.Errorf("add match: %w", err)
		}
	}

	sigCh := make(chan *dbus.Signal, 128)
	conn.Signal(sigCh)
	defer conn.RemoveSignal(sigCh)
	defer m.set(nil, nil)

	src := busSource{conn}
	m.reload(ctx, src)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sig, ok := <-sigCh:
			if !ok {
				return fmt.Errorf("dbus signal channel closed")
			}
			if sig != nil {
				m.handle(ctx, src, sig)
			}
		}
	}
}

func (m *Monitor) set(objs map[dbus.ObjectPath]map[string]props, refs map[string]props) {
	m.mu.Lock()
	m.objs, m.refs = objs, refs
	m.mu.Unlock()
}

// reload reads everything: at start, when ModemManager starts, and when
// a modem comes, goes, or gets other bearers or another SIM.  It not
// running is no modems, not an error.
func (m *Monitor) reload(ctx context.Context, src source) {
	owner, err := src.owner(ctx)
	if err != nil {
		m.owner = ""
		m.set(nil, nil)
		return
	}
	m.owner = owner

	objs, err := src.objects(ctx)
	if err != nil {
		m.set(nil, nil)
		return
	}

	refs := make(map[string]props)
	for _, obj := range objs {
		gen := obj[ifModem]
		paths, _ := gen["Bearers"].Value().([]dbus.ObjectPath)
		for _, path := range paths {
			if p, err := src.getAll(ctx, string(path), ifBearer); err == nil {
				refs[string(path)] = p
			}
		}
		if path := objPath(gen["Sim"]); path != "" {
			if p, err := src.getAll(ctx, path, ifSim); err == nil {
				refs[path] = p
			}
		}
	}
	m.set(objs, refs)
}

// handle applies a signal.  Any client may send one to yangerd, so only
// the bus itself and ModemManager are listened to.
func (m *Monitor) handle(ctx context.Context, src source, sig *dbus.Signal) {
	if sig.Name == dbusName+".NameOwnerChanged" {
		if sig.Sender != dbusName || len(sig.Body) < 3 {
			return
		}
		if owner, _ := sig.Body[2].(string); owner == "" {
			m.owner = ""
			m.set(nil, nil)
		} else {
			m.reload(ctx, src)
		}
		return
	}
	if m.owner == "" || sig.Sender != m.owner {
		return
	}

	switch sig.Name {

	case objectManager + ".InterfacesAdded", objectManager + ".InterfacesRemoved":
		m.reload(ctx, src)

	case properties + ".PropertiesChanged":
		if len(sig.Body) < 3 {
			return
		}
		iface, _ := sig.Body[0].(string)
		changed, _ := sig.Body[1].(map[string]dbus.Variant)
		_, bearers := changed["Bearers"]
		_, sim := changed["Sim"]
		invalidated, _ := sig.Body[2].([]string)
		if iface == ifModem && (bearers || sim) {
			m.reload(ctx, src)
			return
		}
		if len(invalidated) > 0 {
			if p, err := src.getAll(ctx, string(sig.Path), iface); err == nil {
				changed = p
			}
		}
		m.update(sig.Path, iface, changed)
	}
}

// update merges changed properties into a modem, bearer or SIM.
func (m *Monitor) update(path dbus.ObjectPath, iface string, changed props) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if obj, ok := m.objs[path]; ok {
		if obj[iface] == nil {
			obj[iface] = make(props)
		}
		maps.Copy(obj[iface], changed)
	} else if p, ok := m.refs[string(path)]; ok && (iface == ifBearer || iface == ifSim) {
		maps.Copy(p, changed)
	}
}

type busSource struct{ conn *dbus.Conn }

func (b busSource) owner(ctx context.Context) (string, error) {
	var owner string
	err := b.conn.BusObject().CallWithContext(ctx, dbusName+".GetNameOwner", 0,
		service).Store(&owner)
	return owner, err
}

func (b busSource) objects(ctx context.Context) (map[dbus.ObjectPath]map[string]props, error) {
	var objs map[dbus.ObjectPath]map[string]props
	err := b.conn.Object(service, root).CallWithContext(ctx,
		objectManager+".GetManagedObjects", dbus.FlagNoAutoStart).Store(&objs)
	return objs, err
}

func (b busSource) getAll(ctx context.Context, path, iface string) (props, error) {
	var p props
	err := b.conn.Object(service, dbus.ObjectPath(path)).CallWithContext(ctx,
		properties+".GetAll", dbus.FlagNoAutoStart, iface).Store(&p)
	return p, err
}
