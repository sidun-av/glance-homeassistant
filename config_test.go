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
	if cfg.Temperature.ChartHeight != 130 {
		t.Errorf("Temperature.ChartHeight = %d, want 130", cfg.Temperature.ChartHeight)
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

// setEnv sets an env var for the duration of the test and restores whatever
// was there before (unset or a prior value) on cleanup, so env-var tests
// never leak state into sibling tests regardless of run order.
func setEnv(t *testing.T, name, value string) {
	t.Helper()
	prev, had := os.LookupEnv(name)
	if err := os.Setenv(name, value); err != nil {
		t.Fatalf("setenv %s: %v", name, err)
	}
	t.Cleanup(func() {
		if had {
			os.Setenv(name, prev)
		} else {
			os.Unsetenv(name)
		}
	})
}

func TestLoadConfig_HomeAssistantEnvOverrides(t *testing.T) {
	setEnv(t, "HA_URL", "http://ha.example:8123")
	setEnv(t, "HA_TOKEN", "secret-token")

	// No home_assistant block in the file at all — env vars alone must supply it.
	path := writeTempConfig(t, `title: Home`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.HomeAssistant.URL != "http://ha.example:8123" {
		t.Errorf("HomeAssistant.URL = %q, want env override", cfg.HomeAssistant.URL)
	}
	if cfg.HomeAssistant.Token != "secret-token" {
		t.Errorf("HomeAssistant.Token = %q, want env override", cfg.HomeAssistant.Token)
	}
}

func TestLoadConfig_EnvOverrideTakesPriorityOverYAML(t *testing.T) {
	setEnv(t, "TITLE", "Env Title")

	path := writeTempConfig(t, `
home_assistant:
  url: http://homeassistant:8123
  token: test-token
title: YAML Title
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Title != "Env Title" {
		t.Errorf("Title = %q, want env override to win over YAML value", cfg.Title)
	}
}

func TestLoadConfig_EmptyEnvVarDoesNotOverride(t *testing.T) {
	setEnv(t, "TITLE", "")

	path := writeTempConfig(t, `
home_assistant:
  url: http://homeassistant:8123
  token: test-token
title: YAML Title
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Title != "YAML Title" {
		t.Errorf("Title = %q, want YAML value preserved when env var is set to empty", cfg.Title)
	}
}

func TestLoadConfig_EnvOverridesAllFields(t *testing.T) {
	setEnv(t, "HA_URL", "http://env-ha:8123")
	setEnv(t, "HA_TOKEN", "env-token")
	setEnv(t, "PUBLIC_URL", "/ha-widget")
	setEnv(t, "TITLE", "Env Home")
	setEnv(t, "TEMPERATURE_RANGE", "12h")
	setEnv(t, "TEMPERATURE_MAX_POINTS", "30")
	setEnv(t, "TEMPERATURE_CHART_HEIGHT", "50")
	setEnv(t, "TEMPERATURE_CHART_STYLE", "bars")
	setEnv(t, "LIVE_POLL_INTERVAL", "5s")
	setEnv(t, "LIVE_PAUSE_WHEN_HIDDEN", "false")
	setEnv(t, "SENSORS_CONTACT_DEVICE_CLASSES", "door,window")
	setEnv(t, "SENSORS_MOTION_DEVICE_CLASSES", "motion")

	path := writeTempConfig(t, `{}`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.HomeAssistant.URL != "http://env-ha:8123" {
		t.Errorf("HomeAssistant.URL = %q", cfg.HomeAssistant.URL)
	}
	if cfg.HomeAssistant.Token != "env-token" {
		t.Errorf("HomeAssistant.Token = %q", cfg.HomeAssistant.Token)
	}
	if cfg.PublicURL != "/ha-widget" {
		t.Errorf("PublicURL = %q", cfg.PublicURL)
	}
	if cfg.Title != "Env Home" {
		t.Errorf("Title = %q", cfg.Title)
	}
	if cfg.Temperature.Range != "12h" {
		t.Errorf("Temperature.Range = %q", cfg.Temperature.Range)
	}
	if cfg.Temperature.MaxPoints != 30 {
		t.Errorf("Temperature.MaxPoints = %d", cfg.Temperature.MaxPoints)
	}
	if cfg.Temperature.ChartHeight != 50 {
		t.Errorf("Temperature.ChartHeight = %d", cfg.Temperature.ChartHeight)
	}
	if cfg.Temperature.ChartStyle != "bars" {
		t.Errorf("Temperature.ChartStyle = %q", cfg.Temperature.ChartStyle)
	}
	if cfg.Live.PollInterval != "5s" {
		t.Errorf("Live.PollInterval = %q", cfg.Live.PollInterval)
	}
	if cfg.Live.PauseWhenHidden == nil || *cfg.Live.PauseWhenHidden != false {
		t.Errorf("Live.PauseWhenHidden = %v, want false", cfg.Live.PauseWhenHidden)
	}
	if len(cfg.Sensors.ContactDeviceClasses) != 2 || cfg.Sensors.ContactDeviceClasses[0] != "door" || cfg.Sensors.ContactDeviceClasses[1] != "window" {
		t.Errorf("ContactDeviceClasses = %v, want [door window]", cfg.Sensors.ContactDeviceClasses)
	}
	if len(cfg.Sensors.MotionDeviceClasses) != 1 || cfg.Sensors.MotionDeviceClasses[0] != "motion" {
		t.Errorf("MotionDeviceClasses = %v, want [motion]", cfg.Sensors.MotionDeviceClasses)
	}
}

func TestLoadConfig_SensorClassesEnvOverride_TrimsWhitespaceAndDropsEmpties(t *testing.T) {
	setEnv(t, "SENSORS_CONTACT_DEVICE_CLASSES", " door , window ,,")

	path := writeTempConfig(t, `
home_assistant:
  url: http://homeassistant:8123
  token: test-token
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	want := []string{"door", "window"}
	if len(cfg.Sensors.ContactDeviceClasses) != len(want) {
		t.Fatalf("ContactDeviceClasses = %v, want %v", cfg.Sensors.ContactDeviceClasses, want)
	}
	for i, v := range want {
		if cfg.Sensors.ContactDeviceClasses[i] != v {
			t.Errorf("ContactDeviceClasses[%d] = %q, want %q", i, cfg.Sensors.ContactDeviceClasses[i], v)
		}
	}
}

func TestLoadConfig_InvalidMaxPointsEnv(t *testing.T) {
	setEnv(t, "TEMPERATURE_MAX_POINTS", "not-a-number")

	path := writeTempConfig(t, `
home_assistant:
  url: http://homeassistant:8123
  token: test-token
`)
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected error for non-numeric TEMPERATURE_MAX_POINTS, got nil")
	}
}

func TestLoadConfig_InvalidChartHeightEnv(t *testing.T) {
	setEnv(t, "TEMPERATURE_CHART_HEIGHT", "tall")

	path := writeTempConfig(t, `
home_assistant:
  url: http://homeassistant:8123
  token: test-token
`)
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected error for non-numeric TEMPERATURE_CHART_HEIGHT, got nil")
	}
}

func TestLoadConfig_InvalidPauseWhenHiddenEnv(t *testing.T) {
	setEnv(t, "LIVE_PAUSE_WHEN_HIDDEN", "maybe")

	path := writeTempConfig(t, `
home_assistant:
  url: http://homeassistant:8123
  token: test-token
`)
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected error for non-boolean LIVE_PAUSE_WHEN_HIDDEN, got nil")
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

// TestLoadConfig_DockerDefaultFileWithEnvOverrides guards the file the
// Dockerfile bakes in as /config.yml: it must parse as valid (if entirely
// commented-out) YAML, and LoadConfig must succeed against it once HA_URL/
// HA_TOKEN are supplied purely via environment variables — the deployment
// mode a GUI stack manager like Komodo relies on.
func TestLoadConfig_DockerDefaultFileWithEnvOverrides(t *testing.T) {
	setEnv(t, "HA_URL", "http://homeassistant:8123")
	setEnv(t, "HA_TOKEN", "test-token")

	cfg, err := LoadConfig("config.docker-default.yml")
	if err != nil {
		t.Fatalf("LoadConfig(config.docker-default.yml): %v", err)
	}
	if cfg.HomeAssistant.URL != "http://homeassistant:8123" {
		t.Errorf("HomeAssistant.URL = %q", cfg.HomeAssistant.URL)
	}
	if cfg.HomeAssistant.Token != "test-token" {
		t.Errorf("HomeAssistant.Token = %q", cfg.HomeAssistant.Token)
	}
	if cfg.Title != "Home" {
		t.Errorf("Title = %q, want the built-in default", cfg.Title)
	}
}
