package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all yangerd runtime configuration, populated from
// environment variables with sensible defaults.
type Config struct {
	Socket       string
	LogLevel     string
	PollRouting  time.Duration
	PollNTP      time.Duration
	PollHardware time.Duration
	PollSTP      time.Duration
}

// Load reads configuration from the environment.
func Load() *Config {
	return &Config{
		Socket:       envStr("YANGERD_SOCKET", "/run/yangerd.sock"),
		LogLevel:     envStr("YANGERD_LOG_LEVEL", "info"),
		PollRouting:  envDur("YANGERD_POLL_INTERVAL_ROUTING", 10*time.Second),
		PollNTP:      envDur("YANGERD_POLL_INTERVAL_NTP", 60*time.Second),
		PollHardware: envDur("YANGERD_POLL_INTERVAL_HARDWARE", 10*time.Second),
		PollSTP:      envDur("YANGERD_POLL_INTERVAL_STP", 5*time.Second),
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
