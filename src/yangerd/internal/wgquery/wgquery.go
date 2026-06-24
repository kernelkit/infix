package wgquery

import (
	"encoding/json"
	"strconv"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl"
)

func Query(links json.RawMessage) map[string]json.RawMessage {
	wgIfaces := findWireguardIfaces(links)
	if len(wgIfaces) == 0 {
		return nil
	}

	client, err := wgctrl.New()
	if err != nil {
		return nil
	}
	defer client.Close()

	result := make(map[string]json.RawMessage)
	now := time.Now().UTC()

	for _, ifname := range wgIfaces {
		dev, err := client.Device(ifname)
		if err != nil {
			continue
		}
		if len(dev.Peers) == 0 {
			continue
		}

		var peers []map[string]any
		for _, p := range dev.Peers {
			peer := map[string]any{
				"public-key":        p.PublicKey.String(),
				"connection-status": connectionStatus(p.LastHandshakeTime, now),
			}

			if !p.LastHandshakeTime.IsZero() {
				peer["latest-handshake"] = p.LastHandshakeTime.UTC().Format("2006-01-02T15:04:05+00:00")
			}

			if p.Endpoint != nil {
				peer["endpoint-address"] = p.Endpoint.IP.String()
				peer["endpoint-port"] = p.Endpoint.Port
			}

			if p.TransmitBytes > 0 || p.ReceiveBytes > 0 {
				peer["transfer"] = map[string]any{
					"tx-bytes": strconv.FormatInt(p.TransmitBytes, 10),
					"rx-bytes": strconv.FormatInt(p.ReceiveBytes, 10),
				}
			}

			peers = append(peers, peer)
		}

		if len(peers) == 0 {
			continue
		}

		out, err := json.Marshal(map[string]any{"peer-status": map[string]any{"peer": peers}})
		if err != nil {
			continue
		}
		result[ifname] = out
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

func findWireguardIfaces(links json.RawMessage) []string {
	var ifaces []map[string]any
	if json.Unmarshal(links, &ifaces) != nil {
		return nil
	}

	var result []string
	for _, iface := range ifaces {
		linkinfo, _ := iface["linkinfo"].(map[string]any)
		if linkinfo == nil {
			continue
		}
		if kind, _ := linkinfo["info_kind"].(string); kind == "wireguard" {
			if name, _ := iface["ifname"].(string); name != "" {
				result = append(result, name)
			}
		}
	}
	return result
}

func connectionStatus(handshake time.Time, now time.Time) string {
	if handshake.IsZero() {
		return "down"
	}
	if now.Sub(handshake) < 180*time.Second {
		return "up"
	}
	return "down"
}
