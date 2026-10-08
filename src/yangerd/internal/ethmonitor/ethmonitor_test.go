package ethmonitor

import (
	"context"
	"encoding/json"
	"github.com/mdlayher/genetlink"
	"github.com/mdlayher/netlink"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBuildEthernetContainerCopper1G(t *testing.T) {
	data := ethtoolJSON{
		Speed:           1000,
		Duplex:          "Full",
		Port:            "Twisted Pair",
		AutoNegotiation: true,
		SupportedLinkModes: []string{
			"10baseT/Half", "10baseT/Full",
			"100baseT/Half", "100baseT/Full",
			"1000baseT/Full",
		},
		AdvertisedLinkModes: []string{
			"10baseT/Half", "10baseT/Full",
			"100baseT/Half", "100baseT/Full",
			"1000baseT/Full",
		},
	}

	eth, speedBPS := buildEthernetContainer(data)

	if speedBPS != 1_000_000_000 {
		t.Fatalf("speed = %d, want 1000000000", speedBPS)
	}
	if eth["phy-type"] != "ieee802-ethernet-phy-type:phy-type-1000BASE-T" {
		t.Fatalf("phy-type = %v", eth["phy-type"])
	}
	if eth["pmd-type"] != "ieee802-ethernet-phy-type:pmd-type-1000BASE-T" {
		t.Fatalf("pmd-type = %v", eth["pmd-type"])
	}
	if eth["duplex"] != "full" {
		t.Fatalf("duplex = %v", eth["duplex"])
	}
	autoneg := eth["auto-negotiation"].(map[string]any)
	if autoneg["enable"] != true {
		t.Fatal("autoneg should be true")
	}
	// advertised == supported → no advertised-pmd-types key
	if _, ok := autoneg["infix-ethernet-interface:advertised-pmd-types"]; ok {
		t.Fatal("advertised-pmd-types should be suppressed when equal to supported")
	}
}

func TestBuildEthernetContainerFibre10G(t *testing.T) {
	data := ethtoolJSON{
		Speed:               10000,
		Duplex:              "Full",
		Port:                "FIBRE",
		AutoNegotiation:     false,
		SupportedLinkModes:  []string{"10000baseSR/Full"},
		AdvertisedLinkModes: []string{"10000baseSR/Full"},
	}

	eth, speedBPS := buildEthernetContainer(data)

	if speedBPS != 10_000_000_000 {
		t.Fatalf("speed = %d, want 10000000000", speedBPS)
	}
	// Fibre 10G → phy-type 10GBASE-R, no pmd-type from lookup table
	// But exactly one supported mode → pmd-type refined from supported list
	if eth["pmd-type"] != "ieee802-ethernet-phy-type:pmd-type-10GBASE-SR" {
		t.Fatalf("pmd-type = %v, want refined from single supported mode", eth["pmd-type"])
	}
	if eth["phy-type"] != "ieee802-ethernet-phy-type:phy-type-10GBASE-R" {
		t.Fatalf("phy-type = %v", eth["phy-type"])
	}
}

func TestBuildEthernetContainerSpeedUnknown(t *testing.T) {
	data := ethtoolJSON{
		Speed:           ethtoolSpeedUnknown,
		Duplex:          "Unknown! (255)",
		Port:            "Twisted Pair",
		AutoNegotiation: true,
	}

	eth, speedBPS := buildEthernetContainer(data)

	if speedBPS != 0 {
		t.Fatalf("speed = %d, want 0 for unknown", speedBPS)
	}
	if _, ok := eth["speed"]; ok {
		t.Fatal("speed should not be set when unknown")
	}
	if _, ok := eth["phy-type"]; ok {
		t.Fatal("phy-type should not be set when speed unknown")
	}
}

func TestBuildEthernetContainerAdvertisedDiffers(t *testing.T) {
	data := ethtoolJSON{
		Speed:  1000,
		Duplex: "Full",
		Port:   "Twisted Pair",
		SupportedLinkModes: []string{
			"10baseT/Full", "100baseT/Full", "1000baseT/Full",
		},
		AdvertisedLinkModes: []string{"1000baseT/Full"},
	}

	eth, _ := buildEthernetContainer(data)

	autoneg := eth["auto-negotiation"].(map[string]any)
	adv, ok := autoneg["infix-ethernet-interface:advertised-pmd-types"]
	if !ok {
		t.Fatal("advertised-pmd-types should be present when != supported")
	}
	advList := adv.([]string)
	if len(advList) != 1 || advList[0] != "ieee802-ethernet-phy-type:pmd-type-1000BASE-T" {
		t.Fatalf("advertised = %v", advList)
	}
}

func TestEthtoolModesToPMD(t *testing.T) {
	modes := []string{
		"10baseT/Half", "10baseT/Full",
		"1000baseT/Full",
		"Autoneg", "TP",
	}
	got := ethtoolModesToPMD(modes)
	want := []string{
		"ieee802-ethernet-phy-type:pmd-type-10BASE-T",
		"ieee802-ethernet-phy-type:pmd-type-1000BASE-T",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

type recordingRunner struct {
	mu    sync.Mutex
	calls []string
}

func (r *recordingRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.mu.Lock()
	r.calls = append(r.calls, strings.Join(append([]string{name}, args...), " "))
	r.mu.Unlock()
	return []byte(`[{"speed":1000,"duplex":"Full","port":"Twisted Pair","auto-negotiation":true}]`), nil
}

// A link-modes notification must reach the ethtool refresh for the
// interface named by the header ifindex, and land in the callback.
func TestDispatchNotificationRefreshesInterface(t *testing.T) {
	ae := netlink.NewAttributeEncoder()
	ae.Nested(1, func(nae *netlink.AttributeEncoder) error {
		nae.Uint32(nlaHeaderIfindex, 7)
		return nil
	})
	data, err := ae.Encode()
	if err != nil {
		t.Fatal(err)
	}

	runner := &recordingRunner{}
	m := New(slog.Default(), runner)
	m.ifaceName = func(index int) (string, error) {
		if index != 7 {
			t.Fatalf("ifindex = %d, want 7", index)
		}
		return "eth3", nil
	}
	got := make(chan string, 1)
	m.SetOnUpdate(func(ifname string, raw json.RawMessage) {
		if !strings.Contains(string(raw), `"duplex":"full"`) {
			t.Errorf("unexpected ethernet data: %s", raw)
		}
		got <- ifname
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.worker(ctx)

	m.dispatch(genetlink.Message{Header: genetlink.Header{Command: ETHTOOL_MSG_LINKMODES_NTF}, Data: data})
	m.dispatch(genetlink.Message{Header: genetlink.Header{Command: 28}, Data: data}) // not a link notification

	select {
	case ifname := <-got:
		if ifname != "eth3" {
			t.Fatalf("refreshed %q, want eth3", ifname)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("notification never reached the callback")
	}
	time.Sleep(50 * time.Millisecond)
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if len(runner.calls) != 1 || runner.calls[0] != "ethtool --json eth3" {
		t.Fatalf("ethtool calls = %v", runner.calls)
	}
}
