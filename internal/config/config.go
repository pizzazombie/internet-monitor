package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddr         string
	DataDir            string
	CheckInterval      time.Duration
	RequestTimeout     time.Duration
	RetentionDays      int
	ShutdownTimeout    time.Duration
	EnableSpeedTest    bool
	SpeedTestInterval  time.Duration
	SpeedTestBytes     int64
	SpeedTestURL       string
	LANProbeAddress    string
	TCPProbeAddress    string
	HTTPProbeURL       string
	DNSProbeHost       string
	MaxRenderedPoints  int
	MaxRenderedSamples int
}

func Load() Config {
	return Config{
		ListenAddr:         envString("IM_LISTEN_ADDR", ":5555"),
		DataDir:            envString("IM_DATA_DIR", "./data"),
		CheckInterval:      envDuration("IM_CHECK_INTERVAL", 15*time.Second),
		RequestTimeout:     envDuration("IM_REQUEST_TIMEOUT", 3*time.Second),
		RetentionDays:      envInt("IM_RETENTION_DAYS", 180),
		ShutdownTimeout:    envDuration("IM_SHUTDOWN_TIMEOUT", 5*time.Second),
		EnableSpeedTest:    envBool("IM_ENABLE_SPEED_TEST", false),
		SpeedTestInterval:  envDuration("IM_SPEED_TEST_INTERVAL", 6*time.Hour),
		SpeedTestBytes:     envInt64("IM_SPEED_TEST_BYTES", 262144),
		SpeedTestURL:       envString("IM_SPEED_TEST_URL", "https://speed.cloudflare.com/__down?bytes=262144"),
		LANProbeAddress:    envString("IM_LAN_PROBE_ADDRESS", ""),
		TCPProbeAddress:    envString("IM_TCP_PROBE_ADDRESS", "1.1.1.1:443"),
		HTTPProbeURL:       envString("IM_HTTP_PROBE_URL", "https://cp.cloudflare.com/generate_204"),
		DNSProbeHost:       envString("IM_DNS_PROBE_HOST", "example.com"),
		MaxRenderedPoints:  envInt("IM_MAX_RENDERED_POINTS", 720),
		MaxRenderedSamples: envInt("IM_MAX_RENDERED_SAMPLES", 480),
	}
}

func envString(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt64(key string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
