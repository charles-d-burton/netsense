package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type HAConfig struct {
	Name              string `yaml:"name"`
	DeviceClass       string `yaml:"device_class"`
	StateTopic        string `yaml:"state_topic"`
	AvailabilityTopic string `yaml:"availability_topic"`
	UniqueID          string `yaml:"unique_id"`
}

type Filter struct {
	Service    string   `yaml:"service"`
	Direction  string   `yaml:"direction"` // "src", "dst", "both"
	Cidrs      []string `yaml:"cidrs"`
	Portranges []string `yaml:"portranges"`
	Protocols  []string `yaml:"protocols"`
}

type MqttConfig struct {
	Host                string        `yaml:"host"`
	Port                string        `yaml:"port"`
	User                string        `yaml:"user"`
	Password            string        `yaml:"password"`
	ClientID            string        `yaml:"client_id"`
	ConnectTimeout      time.Duration `yaml:"connect_timeout"`
	PingInterval        time.Duration `yaml:"ping_interval"`
	ReconnectBackoff    time.Duration `yaml:"reconnect_backoff"`
	PayloadActive       string        `yaml:"payload_active"`
	PayloadInactive     string        `yaml:"payload_inactive"`
	PayloadAvailable    string        `yaml:"payload_available"`
	PayloadNotAvailable string        `yaml:"payload_not_available"`
	TLS                 bool          `yaml:"tls"`
	InsecureSkipVerify  bool          `yaml:"insecure_skip_verify"`
	CAFile              string        `yaml:"ca_file"`
	CertFile            string        `yaml:"cert_file"`
	KeyFile             string        `yaml:"key_file"`
}

type PcapConfig struct {
	SnapLen         int32         `yaml:"snap_len"`
	Promiscuous     bool          `yaml:"promiscuous"`
	Timeout         time.Duration `yaml:"timeout"`
	ActiveThreshold int           `yaml:"active_threshold"`
	OnDelay         time.Duration `yaml:"on_delay"`
	OffDelay        time.Duration `yaml:"off_delay"`
}

type SensorConfig struct {
	Name          string     `yaml:"name"`
	Homeassistant HAConfig   `yaml:"homeassistant"`
	Filters       []Filter   `yaml:"filters"`
	Pcap          PcapConfig `yaml:"pcap"`
}

type Device struct {
	Name         string   `json:"name" yaml:"name"`
	Identifiers  []string `json:"identifiers" yaml:"identifiers"`
	Model        string   `json:"model" yaml:"model"`
	Manufacturer string   `json:"manufacturer" yaml:"manufacturer"`
}

type Config struct {
	Mqtt    MqttConfig     `yaml:"mqtt"`
	Device  Device         `yaml:"device"`
	Sensors []SensorConfig `yaml:"sensors"`
}

func (c *Config) Validate() error {
	if c.Mqtt.Host == "" || c.Mqtt.Port == "" {
		return errors.New("MQTT host and port are required")
	}
	if len(c.Sensors) == 0 {
		return errors.New("at least one sensor is required")
	}
	for i, s := range c.Sensors {
		if s.Name == "" {
			return fmt.Errorf("sensor %d: name is required", i)
		}
		if s.Homeassistant.StateTopic == "" {
			return fmt.Errorf("sensor %s: home assistant state topic is required", s.Name)
		}
		for _, f := range s.Filters {
			switch f.Direction {
			case "", "src", "dst", "both":
			default:
				return fmt.Errorf("sensor %s: invalid filter direction %s", s.Name, f.Direction)
			}
		}
		if len(s.Filters) == 0 {
			return fmt.Errorf("sensor %s: at least one filter is required", s.Name)
		}
		if s.Pcap.ActiveThreshold < 1 {
			return fmt.Errorf("sensor %s: pcap active threshold must be at least 1", s.Name)
		}
		filter := BuildBPFFilter(s.Filters)
		if err := ValidateBPFFilter(filter); err != nil {
			return fmt.Errorf("sensor %s: invalid BPF filter generated from config: %w", s.Name, err)
		}
	}
	return nil
}

func DefaultSensorConfig(name string) SensorConfig {
	return SensorConfig{
		Name: name,
		Pcap: PcapConfig{
			SnapLen:         262144,
			Promiscuous:     false,
			Timeout:         0,
			ActiveThreshold: 1,
			OnDelay:         0,
			OffDelay:        0,
		},
	}
}

func NewConfig() Config {
	c := Config{}
	c.Mqtt.ConnectTimeout = 4 * time.Second
	c.Mqtt.PingInterval = 30 * time.Second
	c.Mqtt.ReconnectBackoff = 1 * time.Second
	c.Mqtt.PayloadActive = "ON"
	c.Mqtt.PayloadInactive = "OFF"
	c.Mqtt.PayloadAvailable = "online"
	c.Mqtt.PayloadNotAvailable = "offline"
	return c
}

type DiscoveryConfig struct {
	Name              string `json:"name"`
	DeviceClass       string `json:"device_class"`
	StateTopic        string `json:"state_topic"`
	AvailabilityTopic string `json:"availability_topic,omitempty"`
	UniqueID          string `json:"unique_id"`
	Device            Device `json:"device"`
}

func GetDiscoveryConfig(haconfig HAConfig, device Device) ([]byte, error) {
	mc := DiscoveryConfig{
		Name:              haconfig.Name,
		DeviceClass:       haconfig.DeviceClass,
		StateTopic:        haconfig.StateTopic,
		AvailabilityTopic: haconfig.AvailabilityTopic,
		UniqueID:          haconfig.UniqueID,
		Device:            device,
	}
	return json.Marshal(mc)
}

func FindConfigFile() (string, error) {
	locations := []string{
		"config.yaml",
		filepath.Join("/etc", "netsense", "config.yaml"),
	}

	configDir, err := os.UserConfigDir()
	if err == nil {
		locations = append(locations, filepath.Join(configDir, "netsense", "config.yaml"))
	}

	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			return loc, nil
		}
	}

	// If not found, try to create the default /etc/netsense/config.yaml
	defaultPath := filepath.Join("/etc", "netsense", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(defaultPath), 0o755); err == nil {
		if err := WriteDefaultConfig(defaultPath); err == nil {
			slog.Info("created skeleton config", "path", defaultPath)
			return defaultPath, nil
		}
	}

	return "", errors.New("config file not found and could not be created")
}

func WriteDefaultConfig(path string) error {
	c := NewConfig()
	c.Mqtt.Host = "localhost"
	c.Mqtt.Port = "1883"
	c.Device = Device{
		Name:         "NetSense Monitor",
		Model:        "NetSense Monitor",
		Manufacturer: "NetSense",
		Identifiers:  []string{"netsense_monitor"},
	}

	sensor := DefaultSensorConfig("Meeting")
	sensor.Homeassistant.StateTopic = "homeassistant/binary_sensor/meeting/state"
	sensor.Homeassistant.AvailabilityTopic = "homeassistant/binary_sensor/meeting/availability"
	sensor.Filters = []Filter{
		{
			Service:    "Example",
			Direction:  "dst",
			Portranges: []string{"443"},
			Protocols:  []string{"tcp"},
		},
	}
	c.Sensors = append(c.Sensors, sensor)

	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	config := NewConfig()
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	// Ensure defaults for sensors
	for i := range config.Sensors {
		if config.Sensors[i].Pcap.SnapLen == 0 {
			config.Sensors[i].Pcap.SnapLen = 262144
		}
		if config.Sensors[i].Pcap.ActiveThreshold == 0 {
			config.Sensors[i].Pcap.ActiveThreshold = 1
		}
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &config, nil
}
