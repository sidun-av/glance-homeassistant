package main

import (
	"fmt"
	"os"
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

	cfg.HomeAssistant.URL = os.Expand(cfg.HomeAssistant.URL, os.Getenv)
	cfg.HomeAssistant.Token = os.Expand(cfg.HomeAssistant.Token, os.Getenv)

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
		cfg.Temperature.ChartHeight = 34
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
	if _, err := time.ParseDuration(cfg.Temperature.Range); err != nil {
		return nil, fmt.Errorf("temperature.range %q must be a Go duration like \"24h\" or \"6h\": %w", cfg.Temperature.Range, err)
	}
	if _, err := time.ParseDuration(cfg.Live.PollInterval); err != nil {
		return nil, fmt.Errorf("live.poll_interval %q must be a Go duration like \"10s\": %w", cfg.Live.PollInterval, err)
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
