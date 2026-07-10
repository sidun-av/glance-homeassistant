package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempConfig(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestLoadConfig_Defaults(t *testing.T) {
	path := writeTempConfig(t, `
home_assistant:
  url: http://homeassistant:8123
  token: test-token
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Title != "Home" {
		t.Errorf("Title = %q, want %q", cfg.Title, "Home")
	}
	if cfg.PublicURL != "" {
		t.Errorf("PublicURL = %q, want empty (no forced default)", cfg.PublicURL)
	}
	if cfg.Temperature.Range != "24h" {
		t.Errorf("Temperature.Range = %q, want %q", cfg.Temperature.Range, "24h")
	}
	if cfg.Temperature.MaxPoints != 60 {
		t.Errorf("Temperature.MaxPoints = %d, want 60", cfg.Temperature.MaxPoints)
	}
	if cfg.Temperature.ChartHeight != 34 {
		t.Errorf("Temperature.ChartHeight = %d, want 34", cfg.Temperature.ChartHeight)
	}
	if cfg.Temperature.ChartStyle != "sparkline" {
		t.Errorf("Temperature.ChartStyle = %q, want %q", cfg.Temperature.ChartStyle, "sparkline")
	}
	if cfg.Live.PollInterval != "10s" {
		t.Errorf("Live.PollInterval = %q, want %q", cfg.Live.PollInterval, "10s")
	}
	if cfg.Live.PauseWhenHidden == nil || *cfg.Live.PauseWhenHidden != true {
		t.Errorf("Live.PauseWhenHidden = %v, want true", cfg.Live.PauseWhenHidden)
	}
	wantContact := []string{"door", "window", "garage_door", "opening"}
	if len(cfg.Sensors.ContactDeviceClasses) != len(wantContact) {
		t.Errorf("ContactDeviceClasses = %v, want %v", cfg.Sensors.ContactDeviceClasses, wantContact)
	}
	wantMotion := []string{"motion", "occupancy"}
	if len(cfg.Sensors.MotionDeviceClasses) != len(wantMotion) {
		t.Errorf("MotionDeviceClasses = %v, want %v", cfg.Sensors.MotionDeviceClasses, wantMotion)
	}
}

func TestLoadConfig_EnvExpansion(t *testing.T) {
	os.Setenv("TEST_HA_URL", "http://ha.example:8123")
	os.Setenv("TEST_HA_TOKEN", "secret-token")
	defer os.Unsetenv("TEST_HA_URL")
	defer os.Unsetenv("TEST_HA_TOKEN")

	path := writeTempConfig(t, `
home_assistant:
  url: ${TEST_HA_URL}
  token: ${TEST_HA_TOKEN}
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.HomeAssistant.URL != "http://ha.example:8123" {
		t.Errorf("HomeAssistant.URL = %q, want expanded value", cfg.HomeAssistant.URL)
	}
	if cfg.HomeAssistant.Token != "secret-token" {
		t.Errorf("HomeAssistant.Token = %q, want expanded value", cfg.HomeAssistant.Token)
	}
}

func TestLoadConfig_MissingURL(t *testing.T) {
	path := writeTempConfig(t, `
home_assistant:
  token: test-token
`)
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected error for missing home_assistant.url, got nil")
	}
}

func TestLoadConfig_MissingToken(t *testing.T) {
	path := writeTempConfig(t, `
home_assistant:
  url: http://homeassistant:8123
`)
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected error for missing home_assistant.token, got nil")
	}
}

func TestLoadConfig_InvalidRange(t *testing.T) {
	path := writeTempConfig(t, `
home_assistant:
  url: http://homeassistant:8123
  token: test-token
temperature:
  range: 1d
`)
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected error for range \"1d\" (Go duration has no day unit), got nil")
	}
}

func TestLoadConfig_NegativeRange(t *testing.T) {
	path := writeTempConfig(t, `
home_assistant:
  url: http://homeassistant:8123
  token: test-token
temperature:
  range: -1h
`)
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected error for non-positive temperature.range, got nil")
	}
}

func TestLoadConfig_InvalidPollInterval(t *testing.T) {
	path := writeTempConfig(t, `
home_assistant:
  url: http://homeassistant:8123
  token: test-token
live:
  poll_interval: soon
`)
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected error for invalid live.poll_interval, got nil")
	}
}

func TestLoadConfig_ZeroPollInterval(t *testing.T) {
	path := writeTempConfig(t, `
home_assistant:
  url: http://homeassistant:8123
  token: test-token
live:
  poll_interval: 0s
`)
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected error for non-positive live.poll_interval, got nil")
	}
}

func TestLoadConfig_InvalidChartStyle(t *testing.T) {
	path := writeTempConfig(t, `
home_assistant:
  url: http://homeassistant:8123
  token: test-token
temperature:
  chart_style: pie
`)
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected error for invalid temperature.chart_style, got nil")
	}
}

func TestLoadConfig_NegativeMaxPoints(t *testing.T) {
	path := writeTempConfig(t, `
home_assistant:
  url: http://homeassistant:8123
  token: test-token
temperature:
  max_points: -1
`)
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected error for negative max_points, got nil")
	}
}

func TestLoadConfig_CustomSensorClasses(t *testing.T) {
	path := writeTempConfig(t, `
home_assistant:
  url: http://homeassistant:8123
  token: test-token
sensors:
  contact_device_classes: [door]
  motion_device_classes: [motion]
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.Sensors.ContactDeviceClasses) != 1 || cfg.Sensors.ContactDeviceClasses[0] != "door" {
		t.Errorf("ContactDeviceClasses = %v, want [door] (override, not merged with defaults)", cfg.Sensors.ContactDeviceClasses)
	}
	if len(cfg.Sensors.MotionDeviceClasses) != 1 || cfg.Sensors.MotionDeviceClasses[0] != "motion" {
		t.Errorf("MotionDeviceClasses = %v, want [motion]", cfg.Sensors.MotionDeviceClasses)
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	if _, err := LoadConfig("/nonexistent/config.yml"); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadConfig_ExplicitPauseWhenHiddenFalse(t *testing.T) {
	path := writeTempConfig(t, `
home_assistant:
  url: http://homeassistant:8123
  token: test-token
live:
  pause_when_hidden: false
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Live.PauseWhenHidden == nil || *cfg.Live.PauseWhenHidden != false {
		t.Errorf("Live.PauseWhenHidden = %v, want explicit false to be preserved", cfg.Live.PauseWhenHidden)
	}
}
