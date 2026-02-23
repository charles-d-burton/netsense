package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	mqtt "github.com/soypat/natiu-mqtt"
)

// MqttClient interface for testability
type MqttClient interface {
	Connect(ctx context.Context, transport io.ReadWriteCloser, vars *mqtt.VariablesConnect) error
	Disconnect(err error) error
	PublishPayload(flags mqtt.PacketFlags, v mqtt.VariablesPublish, payload []byte) error
	Subscribe(ctx context.Context, v mqtt.VariablesSubscribe) error
	HandleNext() error
	StartPing() error
	IsConnected() bool
}

type MqttManager struct {
	config     MqttConfig
	device     Device
	client     MqttClient
	receivedCh chan []byte
	connectFn  func(context.Context) error
	logger     *slog.Logger
	
	mu         sync.RWMutex
	sensors    []HAConfig
}

func NewMqttManager(config MqttConfig, device Device, logger *slog.Logger) *MqttManager {
	if logger == nil {
		logger = slog.Default()
	}
	
	m := &MqttManager{
		config:     config,
		device:     device,
		receivedCh: make(chan []byte, 10),
		logger:     logger.With("component", "mqtt"),
	}

	m.connectFn = m.connect
	m.client = mqtt.NewClient(mqtt.ClientConfig{
		Decoder: mqtt.DecoderNoAlloc{UserBuffer: make([]byte, 1500)},
		OnPub: func(_ mqtt.Header, _ mqtt.VariablesPublish, r io.Reader) error {
			message, _ := io.ReadAll(r)
			if len(message) > 0 {
				select {
				case m.receivedCh <- message:
				default:
					m.logger.Warn("receiver buffer full, dropping message")
				}
			}
			return nil
		},
	})

	return m
}

func (m *MqttManager) SetClient(client MqttClient) {
	m.client = client
}

func (m *MqttManager) SetConnectFn(fn func(context.Context) error) {
	m.connectFn = fn
}

func (m *MqttManager) RegisterSensor(ha HAConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sensors = append(m.sensors, ha)
}

func (m *MqttManager) connect(ctx context.Context) error {
	address := net.JoinHostPort(m.config.Host, m.config.Port)
	dialer := &net.Dialer{Timeout: m.config.ConnectTimeout}

	var conn net.Conn
	var err error

	if m.config.TLS {
		tlsConfig := &tls.Config{InsecureSkipVerify: m.config.InsecureSkipVerify}
		if m.config.CAFile != "" {
			caCert, err := os.ReadFile(m.config.CAFile)
			if err != nil {
				return fmt.Errorf("failed to read CA: %w", err)
			}
			caCertPool := x509.NewCertPool()
			caCertPool.AppendCertsFromPEM(caCert)
			tlsConfig.RootCAs = caCertPool
		}
		if m.config.CertFile != "" && m.config.KeyFile != "" {
			cert, err := tls.LoadX509KeyPair(m.config.CertFile, m.config.KeyFile)
			if err != nil {
				return fmt.Errorf("failed to load cert/key: %w", err)
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
		}
		m.logger.Info("connecting via TLS", "address", address)
		conn, err = tls.DialWithDialer(dialer, "tcp", address, tlsConfig)
	} else {
		m.logger.Info("connecting", "address", address)
		conn, err = dialer.DialContext(ctx, "tcp", address)
	}

	if err != nil {
		return err
	}

	var varConn mqtt.VariablesConnect
	varConn.SetDefaultMQTT([]byte(m.config.ClientID))
	varConn.Username = []byte(m.config.User)
	varConn.Password = []byte(m.config.Password)
	
	m.mu.RLock()
	if len(m.sensors) > 0 && m.sensors[0].AvailabilityTopic != "" {
		varConn.WillTopic = []byte(m.sensors[0].AvailabilityTopic)
		varConn.WillMessage = []byte(m.config.PayloadNotAvailable)
		varConn.WillRetain = true
	}
	m.mu.RUnlock()

	connCtx, cancel := context.WithTimeout(ctx, m.config.ConnectTimeout)
	defer cancel()
	return m.client.Connect(connCtx, conn, &varConn)
}

func (m *MqttManager) Start(ctx context.Context) error {
	if err := m.connectFn(ctx); err != nil {
		return err
	}

	m.publishAllDiscoveryConfigs()
	m.publishAllAvailabilities(true)
	m.subscribeToAllStates()

	go func() {
		ticker := time.NewTicker(m.config.PingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if m.client.IsConnected() {
					if err := m.client.StartPing(); err != nil {
						m.logger.Error("ping failed", "error", err)
						m.client.Disconnect(err)
					}
				}
			}
		}
	}()

	go func() {
		attempts := 0
		for {
			select {
			case <-ctx.Done():
				if m.client.IsConnected() {
					m.client.Disconnect(errors.New("shutdown"))
				}
				return
			default:
				if !m.client.IsConnected() {
					backoff := exponentialBackoff(attempts, m.config.ReconnectBackoff, maxReconnectBackoff)
					m.logger.Info("reconnecting", "backoff", backoff, "attempt", attempts+1)
					
					timer := time.NewTimer(backoff)
					select {
					case <-ctx.Done():
						timer.Stop()
						return
					case <-timer.C:
					}

					if err := m.connectFn(ctx); err != nil {
						attempts++
						continue
					}
					attempts = 0
					m.publishAllDiscoveryConfigs()
					m.publishAllAvailabilities(true)
					m.subscribeToAllStates()
				}
				if err := m.client.HandleNext(); err != nil {
					m.logger.Warn("connection error", "error", err)
					m.client.Disconnect(err)
				}
			}
		}
	}()

	return nil
}

func (m *MqttManager) publishAllDiscoveryConfigs() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, ha := range m.sensors {
		_ = m.publishDiscoveryConfig(ha)
	}
}

func (m *MqttManager) publishAllAvailabilities(available bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, ha := range m.sensors {
		if ha.AvailabilityTopic != "" {
			m.publishAvailability(ha, available)
		}
	}
}

func (m *MqttManager) publishAvailability(ha HAConfig, available bool) {
	payload := m.config.PayloadNotAvailable
	if available {
		payload = m.config.PayloadAvailable
	}
	
	pflags, _ := mqtt.NewPublishFlags(mqtt.QoS0, false, true)
	vpub := mqtt.VariablesPublish{
		TopicName:        []byte(ha.AvailabilityTopic),
		PacketIdentifier: randInt16(),
	}
	_ = m.client.PublishPayload(pflags, vpub, []byte(payload))
}

func (m *MqttManager) subscribeToAllStates() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, ha := range m.sensors {
		_ = m.subscribeToState(ha.StateTopic)
	}
}

func (m *MqttManager) publishDiscoveryConfig(haConfig HAConfig) error {
	config, err := GetDiscoveryConfig(haConfig, m.device)
	if err != nil {
		return err
	}
	configPflags, _ := mqtt.NewPublishFlags(mqtt.QoS0, false, true)
	vpub := mqtt.VariablesPublish{
		TopicName:        []byte(strings.Replace(haConfig.StateTopic, "/state", "/config", 1)),
		PacketIdentifier: randInt16(),
	}
	return m.client.PublishPayload(configPflags, vpub, config)
}

func (m *MqttManager) PublishState(ha HAConfig, active bool) {
	payload := m.config.PayloadInactive
	if active {
		payload = m.config.PayloadActive
	}

	pflags, _ := mqtt.NewPublishFlags(mqtt.QoS0, false, false)
	vpub := mqtt.VariablesPublish{
		TopicName:        []byte(ha.StateTopic),
		PacketIdentifier: randInt16(),
	}

	if m.client.IsConnected() {
		m.logger.Info("publishing state", "sensor", ha.Name, "state", payload)
		_ = m.client.PublishPayload(pflags, vpub, []byte(payload))
	}
}

func (m *MqttManager) subscribeToState(topic string) error {
	var vsub mqtt.VariablesSubscribe
	vsub.TopicFilters = []mqtt.SubscribeRequest{{TopicFilter: []byte(topic), QoS: mqtt.QoS0}}
	vsub.PacketIdentifier = randInt16()
	return m.client.Subscribe(context.Background(), vsub)
}

func (m *MqttManager) Receive() <-chan []byte {
	return m.receivedCh
}
