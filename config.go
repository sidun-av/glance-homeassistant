package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	HomeAssistant HomeAssistantConfig `yaml:"home_assistant"`
	PublicURL     string              `yaml:"public_url"`
	Title         string              `yaml:"title"`
	Temperature   TemperatureConfig   `yaml:"temperature"`
	Live          LiveConfig          `yaml:"live"`
	Sensors       SensorsConfig       `yaml:"sensors"`
}

type HomeAssistantConfig struct {
	URL   string `yaml:"url"`
	Token string `yaml:"token"`
}

type TemperatureConfig struct {
	Range       string `yaml:"range"`
	MaxPoints   int    `yaml:"max_points"`
	ChartHeight int    `yaml:"chart_height"`
	ChartStyle  string `yaml:"chart_style"`
}

type LiveConfig struct {
	PollInterval    string `yaml:"poll_interval"`
	PauseWhenHidden *bool  `yaml:"pause_when_hidden"`
}

type SensorsConfig struct {
	ContactDeviceClasses []string `yaml:"contact_device_classes"`
	MotionDeviceClasses  []string `yaml:"motion_device_classes"`
}

func LoadConfig(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := applyEnvOverrides(&cfg); err != nil {
		return nil, err
	}

	if cfg.Title == "" {
		cfg.Title = "Home"
	}
	if cfg.Temperature.Range == "" {
		cfg.Temperature.Range = "24h"
	}
	if cfg.Temperature.MaxPoints == 0 {
		cfg.Temperature.MaxPoints = 60
	}
	if cfg.Temperature.ChartHeight == 0 {
		cfg.Temperature.ChartHeight = 130
	}
	if cfg.Temperature.ChartStyle == "" {
		cfg.Temperature.ChartStyle = "sparkline"
	}
	if cfg.Live.PollInterval == "" {
		cfg.Live.PollInterval = "10s"
	}
	if cfg.Live.PauseWhenHidden == nil {
		t := true
		cfg.Live.PauseWhenHidden = &t
	}
	if len(cfg.Sensors.ContactDeviceClasses) == 0 {
		cfg.Sensors.ContactDeviceClasses = []string{"door", "window", "garage_door", "opening"}
	}
	if len(cfg.Sensors.MotionDeviceClasses) == 0 {
		cfg.Sensors.MotionDeviceClasses = []string{"motion", "occupancy"}
	}

	if cfg.HomeAssistant.URL == "" {
		return nil, fmt.Errorf("home_assistant.url is required")
	}
	if cfg.HomeAssistant.Token == "" {
		return nil, fmt.Errorf("home_assistant.token is required")
	}
	if d, err := time.ParseDuration(cfg.Temperature.Range); err != nil {
		return nil, fmt.Errorf("temperature.range %q must be a Go duration like \"24h\" or \"6h\": %w", cfg.Temperature.Range, err)
	} else if d <= 0 {
		return nil, fmt.Errorf("temperature.range must be positive, got %q", cfg.Temperature.Range)
	}
	if d, err := time.ParseDuration(cfg.Live.PollInterval); err != nil {
		return nil, fmt.Errorf("live.poll_interval %q must be a Go duration like \"10s\": %w", cfg.Live.PollInterval, err)
	} else if d <= 0 {
		return nil, fmt.Errorf("live.poll_interval must be positive, got %q", cfg.Live.PollInterval)
	}
	if cfg.Temperature.ChartStyle != "sparkline" && cfg.Temperature.ChartStyle != "bars" {
		return nil, fmt.Errorf("temperature.chart_style must be \"sparkline\" or \"bars\", got %q", cfg.Temperature.ChartStyle)
	}
	if cfg.Temperature.MaxPoints < 0 {
		return nil, fmt.Errorf("temperature.max_points must not be negative, got %d", cfg.Temperature.MaxPoints)
	}
	if cfg.Temperature.ChartHeight < 0 {
		return nil, fmt.Errorf("temperature.chart_height must not be negative, got %d", cfg.Temperature.ChartHeight)
	}

	return &cfg, nil
}

// lookupNonEmptyEnv returns (value, true) only when the environment variable
// is actually set to a non-empty string. This treats "not set" and "set to
// empty" the same way: as "don't override" — which matters for GUI-driven
// deployments (e.g. Komodo) where an unfilled-in stack variable is passed
// through to the container as an empty string rather than being absent.
func lookupNonEmptyEnv(name string) (string, bool) {
	v, ok := os.LookupEnv(name)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

// splitEnvList parses a comma-separated environment variable value into a
// slice, trimming whitespace around each entry and dropping empty entries
// (so a trailing comma or extra spaces don't produce blank list items).
func splitEnvList(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// applyEnvOverrides layers environment variables on top of whatever was
// loaded from config.yml, so every setting can be driven entirely by
// environment variables (e.g. from a Komodo stack's Environment tab) with no
// need to mount or edit a config file at all. An env var only takes effect
// when set to a non-empty value; anything left unset falls through to the
// YAML file's value, and ultimately to LoadConfig's own defaults below.
func applyEnvOverrides(cfg *Config) error {
	if v, ok := lookupNonEmptyEnv("HA_URL"); ok {
		cfg.HomeAssistant.URL = v
	}
	if v, ok := lookupNonEmptyEnv("HA_TOKEN"); ok {
		cfg.HomeAssistant.Token = v
	}
	if v, ok := lookupNonEmptyEnv("PUBLIC_URL"); ok {
		cfg.PublicURL = v
	}
	if v, ok := lookupNonEmptyEnv("TITLE"); ok {
		cfg.Title = v
	}
	if v, ok := lookupNonEmptyEnv("TEMPERATURE_RANGE"); ok {
		cfg.Temperature.Range = v
	}
	if v, ok := lookupNonEmptyEnv("TEMPERATURE_MAX_POINTS"); ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("env TEMPERATURE_MAX_POINTS=%q is not a valid integer: %w", v, err)
		}
		cfg.Temperature.MaxPoints = n
	}
	if v, ok := lookupNonEmptyEnv("TEMPERATURE_CHART_HEIGHT"); ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("env TEMPERATURE_CHART_HEIGHT=%q is not a valid integer: %w", v, err)
		}
		cfg.Temperature.ChartHeight = n
	}
	if v, ok := lookupNonEmptyEnv("TEMPERATURE_CHART_STYLE"); ok {
		cfg.Temperature.ChartStyle = v
	}
	if v, ok := lookupNonEmptyEnv("LIVE_POLL_INTERVAL"); ok {
		cfg.Live.PollInterval = v
	}
	if v, ok := lookupNonEmptyEnv("LIVE_PAUSE_WHEN_HIDDEN"); ok {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("env LIVE_PAUSE_WHEN_HIDDEN=%q is not a valid boolean: %w", v, err)
		}
		cfg.Live.PauseWhenHidden = &b
	}
	if v, ok := lookupNonEmptyEnv("SENSORS_CONTACT_DEVICE_CLASSES"); ok {
		cfg.Sensors.ContactDeviceClasses = splitEnvList(v)
	}
	if v, ok := lookupNonEmptyEnv("SENSORS_MOTION_DEVICE_CLASSES"); ok {
		cfg.Sensors.MotionDeviceClasses = splitEnvList(v)
	}
	return nil
}
