package mmmonitor

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"maps"
	"testing"

	"github.com/godbus/dbus/v5"
)

const (
	modem0  = dbus.ObjectPath("/org/freedesktop/ModemManager1/Modem/0")
	bearer0 = "/org/freedesktop/ModemManager1/Bearer/0"
	bearer1 = "/org/freedesktop/ModemManager1/Bearer/1"
	sim0    = "/org/freedesktop/ModemManager1/SIM/0"
)

// fakeBus is ModemManager with one modem, or none when down.
type fakeBus struct {
	down  bool
	objs  map[dbus.ObjectPath]map[string]props
	refs  map[string]props
	reads []string // GetAll calls
}

const mmOwner = ":1.5"

func (f *fakeBus) owner(context.Context) (string, error) {
	if f.down {
		return "", errors.New("org.freedesktop.DBus.Error.NameHasNoOwner")
	}
	return mmOwner, nil
}

func (f *fakeBus) objects(context.Context) (map[dbus.ObjectPath]map[string]props, error) {
	if f.down {
		return nil, errors.New("org.freedesktop.DBus.Error.ServiceUnknown")
	}
	return f.objs, nil
}

func (f *fakeBus) getAll(_ context.Context, path, _ string) (props, error) {
	f.reads = append(f.reads, path)
	if p, ok := f.refs[path]; ok {
		return p, nil
	}
	return nil, errors.New("no such object")
}

func newFakeBus() *fakeBus {
	return &fakeBus{
		objs: map[dbus.ObjectPath]map[string]props{
			modem0: {ifModem: {
				"Device":  dbus.MakeVariant("/sys/devices/usb1/1-1/1-1.1"),
				"State":   dbus.MakeVariant(int32(11)),
				"Bearers": dbus.MakeVariant([]dbus.ObjectPath{bearer0}),
				"Sim":     dbus.MakeVariant(dbus.ObjectPath(sim0)),
			}},
		},
		refs: map[string]props{
			bearer0: {"Interface": dbus.MakeVariant("wwan0"), "Connected": dbus.MakeVariant(true)},
			bearer1: {"Interface": dbus.MakeVariant("wwan1"), "Connected": dbus.MakeVariant(false)},
			sim0:    {"OperatorName": dbus.MakeVariant("Fello")},
		},
	}
}

func newMonitor(t *testing.T, bus *fakeBus) *Monitor {
	t.Helper()
	m := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	m.reload(context.Background(), bus)
	return m
}

// newSignal is a signal from ModemManager, or from the bus itself for
// NameOwnerChanged.
func newSignal(path dbus.ObjectPath, name string, body ...interface{}) *dbus.Signal {
	sender := mmOwner
	if name == dbusName+".NameOwnerChanged" {
		sender = dbusName
	}
	return &dbus.Signal{Sender: sender, Path: path, Name: name, Body: body}
}

func onlyModem(t *testing.T, m *Monitor) Modem {
	t.Helper()
	modems := m.Modems()
	if len(modems) != 1 {
		t.Fatalf("got %d modems, want 1", len(modems))
	}
	return modems[0]
}

func TestMonitorReload(t *testing.T) {
	got := onlyModem(t, newMonitor(t, newFakeBus()))
	if got.State != 11 || len(got.Bearers) != 1 || got.Bearers[0].Interface != "wwan0" {
		t.Errorf("modem = %+v", got)
	}
	if got.Sim == nil || got.Sim.OperatorName != "Fello" {
		t.Errorf("sim = %+v", got.Sim)
	}
}

func TestMonitorNotRunning(t *testing.T) {
	bus := newFakeBus()
	bus.down = true
	if got := newMonitor(t, bus).Modems(); len(got) != 0 {
		t.Errorf("got %d modems with ModemManager down", len(got))
	}
}

// A changed property is taken from the signal, nothing is read back.
func TestMonitorPropertiesChanged(t *testing.T) {
	bus := newFakeBus()
	m := newMonitor(t, bus)
	reads := len(bus.reads)

	m.handle(context.Background(), bus, newSignal(modem0, properties+".PropertiesChanged",
		ifModem, map[string]dbus.Variant{"State": dbus.MakeVariant(int32(8))}, []string{}))
	m.handle(context.Background(), bus, newSignal(dbus.ObjectPath(sim0), properties+".PropertiesChanged",
		ifSim, map[string]dbus.Variant{"OperatorName": dbus.MakeVariant("Telia")}, []string{}))
	m.handle(context.Background(), bus, newSignal(modem0, properties+".PropertiesChanged",
		if3gpp, map[string]dbus.Variant{"OperatorName": dbus.MakeVariant("Telia")}, []string{}))

	got := onlyModem(t, m)
	if got.State != 8 || got.Sim.OperatorName != "Telia" || got.OperatorName != "Telia" {
		t.Errorf("modem = %+v, sim %+v", got, got.Sim)
	}
	if len(bus.reads) != reads {
		t.Errorf("read %v, want nothing", bus.reads[reads:])
	}
}

// A modem with a new bearer has it read, the old one is forgotten.
func TestMonitorBearersChanged(t *testing.T) {
	bus := newFakeBus()
	m := newMonitor(t, bus)

	bearers := map[string]dbus.Variant{"Bearers": dbus.MakeVariant([]dbus.ObjectPath{bearer1})}
	maps.Copy(bus.objs[modem0][ifModem], bearers)
	m.handle(context.Background(), bus, newSignal(modem0, properties+".PropertiesChanged",
		ifModem, bearers, []string{}))

	got := onlyModem(t, m)
	if len(got.Bearers) != 1 || got.Bearers[0].Interface != "wwan1" {
		t.Errorf("bearers = %+v", got.Bearers)
	}
	if _, ok := m.refs[bearer0]; ok {
		t.Error("old bearer kept")
	}
}

func TestMonitorInterfaces(t *testing.T) {
	bus := newFakeBus()
	m := newMonitor(t, bus)

	removed := bus.objs[modem0]
	delete(bus.objs, modem0)
	m.handle(context.Background(), bus, newSignal(root, objectManager+".InterfacesRemoved",
		modem0, []string{ifModem, if3gpp}))
	if got := m.Modems(); len(got) != 0 || len(m.refs) != 0 {
		t.Fatalf("modems %v refs %v after removal", got, m.refs)
	}

	bus.objs[modem0] = removed
	m.handle(context.Background(), bus, newSignal(root, objectManager+".InterfacesAdded",
		modem0, map[string]map[string]dbus.Variant(removed)))
	if got := onlyModem(t, m); got.Sim == nil || len(got.Bearers) != 1 {
		t.Errorf("modem = %+v after it came back", got)
	}
}

func TestMonitorNameOwner(t *testing.T) {
	bus := newFakeBus()
	m := newMonitor(t, bus)

	m.handle(context.Background(), bus, newSignal("/org/freedesktop/DBus", dbusName+".NameOwnerChanged",
		service, ":1.5", ""))
	if got := m.Modems(); len(got) != 0 {
		t.Fatalf("got %d modems after ModemManager left", len(got))
	}

	m.handle(context.Background(), bus, newSignal("/org/freedesktop/DBus", dbusName+".NameOwnerChanged",
		service, "", ":1.9"))
	onlyModem(t, m)
}

// Any client can send yangerd a signal, only ModemManager's and the
// bus's own are believed.
func TestMonitorSpoofedSender(t *testing.T) {
	bus := newFakeBus()
	m := newMonitor(t, bus)

	fake := newSignal(modem0, properties+".PropertiesChanged",
		ifModem, map[string]dbus.Variant{"State": dbus.MakeVariant(int32(-1))}, []string{})
	fake.Sender = ":1.66"
	m.handle(context.Background(), bus, fake)

	gone := newSignal("/org/freedesktop/DBus", dbusName+".NameOwnerChanged", service, mmOwner, "")
	gone.Sender = ":1.66"
	m.handle(context.Background(), bus, gone)

	if got := onlyModem(t, m); got.State != 11 {
		t.Errorf("State = %d, a spoofed signal was applied", got.State)
	}
}
