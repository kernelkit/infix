package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all yangerd runtime configuration, populated from
// environment variables with sensible defaults.
type Config struct {
	Socket           string
	LogLevel         string
	ZebraSocket      string
	LLDPCommand      string
	StartupTimeout   time.Duration
	PollSystem       time.Duration
	PollRouting      time.Duration
	PollNTP          time.Duration
	PollHardware     time.Duration
	PollContainers   time.Duration
	EnableWifi       bool
	EnableLLDP       bool
	EnableFirewall   bool
	EnableDHCP       bool
	EnableContainers bool
	EnableGPS        bool
}

// Load reads configuration from the environment.
func Load() *Config {
	return &Config{
		Socket:           envStr("YANGERD_SOCKET", "/run/yangerd.sock"),
		LogLevel:         envStr("YANGERD_LOG_LEVEL", "info"),
		ZebraSocket:      envStr("YANGERD_ZEBRA_SOCKET", "/var/run/frr/zserv.api"),
		LLDPCommand:      envStr("YANGERD_LLDP_COMMAND", "lldpcli"),
		StartupTimeout:   envDur("YANGERD_STARTUP_TIMEOUT", 5*time.Second),
		PollSystem:       envDur("YANGERD_POLL_INTERVAL_SYSTEM", 60*time.Second),
		PollRouting:      envDur("YANGERD_POLL_INTERVAL_ROUTING", 10*time.Second),
		PollNTP:          envDur("YANGERD_POLL_INTERVAL_NTP", 60*time.Second),
		PollHardware:     envDur("YANGERD_POLL_INTERVAL_HARDWARE", 10*time.Second),
		PollContainers:   envDur("YANGERD_POLL_INTERVAL_CONTAINERS", 10*time.Second),
		EnableWifi:       envBool("YANGERD_ENABLE_WIFI", false),
		EnableLLDP:       envBool("YANGERD_ENABLE_LLDP", true),
		EnableFirewall:   envBool("YANGERD_ENABLE_FIREWALL", true),
		EnableDHCP:       envBool("YANGERD_ENABLE_DHCP", true),
		EnableContainers: envBool("YANGERD_ENABLE_CONTAINERS", false),
		EnableGPS:        envBool("YANGERD_ENABLE_GPS", false),
	}
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func envDur(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}
