package main

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestGetDiscoveryConfig(t *testing.T) {
	haConfig := HAConfig{
		Name:        "test_sensor",
		DeviceClass: "connectivity",
		StateTopic:  "homeassistant/binary_sensor/test/state",
		UniqueID:    "test_id_01",
	}

	device := Device{
		Name:         "test_device",
		Model:        "test_model",
		Manufacturer: "test_manufacturer",
		Identifiers:  []string{"id_01", "id_02"},
	}

	expectedConfig := DiscoveryConfig{
		Name:        "test_sensor",
		DeviceClass: "connectivity",
		StateTopic:  "homeassistant/binary_sensor/test/state",
		UniqueID:    "test_id_01",
		Device: Device{
			Name:         "test_device",
			Model:        "test_model",
			Manufacturer: "test_manufacturer",
			Identifiers:  []string{"id_01", "id_02"},
		},
	}
	actualJSON, err := GetDiscoveryConfig(haConfig, device)
	if err != nil {
		t.Fatalf("GetDiscoveryConfig returned an error: %v", err)
	}

	var actualConfig DiscoveryConfig
	err = json.Unmarshal(actualJSON, &actualConfig)
	if err != nil {
		t.Fatalf("Failed to unmarshal actual JSON: %v", err)
	}

	if !reflect.DeepEqual(actualConfig, expectedConfig) {
		t.Errorf("GetDiscoveryConfig returned unexpected config.\nExpected: %+v\nGot: %+v", expectedConfig, actualConfig)
	}
}

func TestConfigValidation(t *testing.T) {
	c := NewConfig()
	c.Mqtt.Host = "localhost"
	c.Mqtt.Port = "1883"
	
	sensor := DefaultSensorConfig("test")
	sensor.Homeassistant.StateTopic = "test/topic"
	sensor.Filters = []Filter{{Service: "test", Cidrs: []string{"192.168.1.0/24"}}}
	c.Sensors = append(c.Sensors, sensor)

	if err := c.Validate(); err != nil {
		t.Errorf("expected valid config, got error: %v", err)
	}
}

func TestNetSenseLifecycle(t *testing.T) {
	// create a temp config file
	tmpfile, err := os.CreateTemp("", "config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	cfg := NewConfig()
	cfg.Mqtt.Host = "localhost"
	cfg.Mqtt.Port = "1883"
	
	sensor := DefaultSensorConfig("test")
	sensor.Homeassistant.StateTopic = "topic"
	sensor.Filters = []Filter{{Service: "test", Cidrs: []string{"192.168.1.0/24"}}}
	cfg.Sensors = append(cfg.Sensors, sensor)
	
	data, _ := yaml.Marshal(cfg)
	if _, err := tmpfile.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	ns, err := NewNetSense(tmpfile.Name())
	if err != nil {
		t.Fatalf("NewNetSense failed: %v", err)
	}
	// Mock the MQTT client so it doesn't actually try to connect during the test
	ns.Mqtt.SetClient(&MockMqttClient{Connected: true})
	ns.Mqtt.SetConnectFn(func(ctx context.Context) error {
		return nil
	})

	// Test cancellation
	go func() {
		time.Sleep(100 * time.Millisecond)
		ns.Cancel()
	}()

	err = ns.Run()
	if err != nil && err != context.Canceled {
		t.Errorf("Run returned unexpected error: %v", err)
	}

	if ns.Config.Mqtt.Host != "localhost" {
		t.Errorf("Config not loaded correctly")
	}
}

func TestRandInt16(t *testing.T) {
	for i := 0; i < 1000; i++ {
		val := randInt16()
		if val == 0 {
			t.Errorf("randInt16 generated 0")
		}
	}
}
