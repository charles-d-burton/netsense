package main

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLoadConfig(t *testing.T) {
	configStr := `
mqtt:
  host: "localhost"
  port: "1883"
device:
  name: "Test Device"
  model: "Test Model"
  identifiers: ["test_id"]
sensors:
  - name: "Test Sensor"
    homeassistant:
      state_topic: "test/topic"
      unique_id: "test_uid"
    filters:
      - service: "test"
        protocols: ["tcp"]
`
	tmpfile, err := os.CreateTemp("", "config_test.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(configStr)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Device.Name != "Test Device" {
		t.Errorf("Expected device name 'Test Device', got '%s'", cfg.Device.Name)
	}
	if len(cfg.Sensors) != 1 {
		t.Errorf("Expected 1 sensor, got %d", len(cfg.Sensors))
	}
	if cfg.Sensors[0].Name != "Test Sensor" {
		t.Errorf("Expected sensor name 'Test Sensor', got '%s'", cfg.Sensors[0].Name)
	}
}

func TestWriteDefaultConfig(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "default_config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	err = WriteDefaultConfig(tmpfile.Name())
	if err != nil {
		t.Fatalf("WriteDefaultConfig failed: %v", err)
	}

	data, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}

	var cfg Config
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		t.Fatalf("Failed to unmarshal default config: %v", err)
	}

	if cfg.Device.Name == "" {
		t.Error("Default config should have a device name")
	}
	if len(cfg.Sensors) == 0 {
		t.Error("Default config should have at least one sensor")
	}
}
