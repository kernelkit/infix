package iwmonitor

import "github.com/kernelkit/infix/src/yangerd/internal/wpactrl"

// channelEvent tells whether a hostapd or wpa_supplicant event means
// the radio went on, off or to another channel.
func channelEvent(ev wpactrl.Event) bool {
	switch ev.Name {
	case "AP-ENABLED", "AP-DISABLED", "AP-CSA-FINISHED",
		"CTRL-EVENT-CONNECTED", "CTRL-EVENT-DISCONNECTED",
		"CTRL-EVENT-CHANNEL-SWITCH", "MESH-GROUP-STARTED", "MESH-GROUP-REMOVED":
		return true
	}
	return false
}
