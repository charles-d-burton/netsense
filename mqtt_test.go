package main

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	mqtt "github.com/soypat/natiu-mqtt"
)

// MockMqttClient is a mock implementation of the MqttClient interface
type MockMqttClient struct {
	Connected         bool
	PublishedPayloads [][]byte
	SubscribedTopics  [][]byte
	FailPublish       bool
	FailSubscribe     bool
	FailConnect       bool
}

func (m *MockMqttClient) Connect(ctx context.Context, transport io.ReadWriteCloser, vars *mqtt.VariablesConnect) error {
	if m.FailConnect {
		return errors.New("mock connect failure")
	}
	m.Connected = true
	return nil
}

func (m *MockMqttClient) Disconnect(err error) error {
	m.Connected = false
	return nil
}

func (m *MockMqttClient) PublishPayload(flags mqtt.PacketFlags, v mqtt.VariablesPublish, payload []byte) error {
	if m.FailPublish {
		return errors.New("mock publish failure")
	}
	m.PublishedPayloads = append(m.PublishedPayloads, payload)
	return nil
}

func (m *MockMqttClient) Subscribe(ctx context.Context, v mqtt.VariablesSubscribe) error {
	if m.FailSubscribe {
		return errors.New("mock subscribe failure")
	}
	for _, req := range v.TopicFilters {
		m.SubscribedTopics = append(m.SubscribedTopics, req.TopicFilter)
	}
	return nil
}

func (m *MockMqttClient) HandleNext() error {
	return nil
}

func (m *MockMqttClient) StartPing() error {
	return nil
}

func (m *MockMqttClient) IsConnected() bool {
	return m.Connected
}

func TestMqttManager_PublishDiscoveryConfig(t *testing.T) {
	mqttConfig := MqttConfig{
		Host: "localhost",
		Port: "1883",
	}
	haConfig := HAConfig{
		Name:       "test",
		StateTopic: "test/state",
	}
	manager := NewMqttManager(mqttConfig, Device{}, nil)
	mockClient := &MockMqttClient{Connected: true}
	manager.client = mockClient

	err := manager.publishDiscoveryConfig(haConfig)
	if err != nil {
		t.Errorf("publishDiscoveryConfig failed: %v", err)
	}

	if len(mockClient.PublishedPayloads) != 1 {
		t.Errorf("Expected 1 published payload, got %d", len(mockClient.PublishedPayloads))
	}
}

func TestMqttManager_SubscribeToState(t *testing.T) {
	mqttConfig := MqttConfig{
		Host: "localhost",
		Port: "1883",
	}
	manager := NewMqttManager(mqttConfig, Device{}, nil)
	mockClient := &MockMqttClient{Connected: true}
	manager.client = mockClient

	err := manager.subscribeToState("test/topic")
	if err != nil {
		t.Errorf("subscribeToState failed: %v", err)
	}

	if len(mockClient.SubscribedTopics) != 1 {
		t.Errorf("Expected 1 subscription, got %d", len(mockClient.SubscribedTopics))
	}
	if string(mockClient.SubscribedTopics[0]) != "test/topic" {
		t.Errorf("Expected subscription to 'test/topic', got '%s'", string(mockClient.SubscribedTopics[0]))
	}
}

func TestMqttManager_StatePublishing(t *testing.T) {
	mqttConfig := MqttConfig{
		ReconnectBackoff: 1 * time.Millisecond,
		PayloadActive:    "ON",
		PayloadInactive:  "OFF",
	}
	haConfig := HAConfig{
		Name: "test",
		StateTopic: "test/topic",
	}
	manager := NewMqttManager(mqttConfig, Device{}, nil)
	mockClient := &MockMqttClient{Connected: true}
	manager.client = mockClient

	manager.PublishState(haConfig, true)

	if len(mockClient.PublishedPayloads) < 1 {
		t.Fatal("Expected payload to be published")
	}
	if string(mockClient.PublishedPayloads[len(mockClient.PublishedPayloads)-1]) != mqttConfig.PayloadActive {
		t.Errorf("Expected last payload to be %s, got %s", mqttConfig.PayloadActive, string(mockClient.PublishedPayloads[len(mockClient.PublishedPayloads)-1]))
	}

	manager.PublishState(haConfig, false)

	if len(mockClient.PublishedPayloads) < 2 {
		t.Fatal("Expected second payload")
	}
	if string(mockClient.PublishedPayloads[len(mockClient.PublishedPayloads)-1]) != mqttConfig.PayloadInactive {
		t.Errorf("Expected last payload to be %s, got %s", mqttConfig.PayloadInactive, string(mockClient.PublishedPayloads[len(mockClient.PublishedPayloads)-1]))
	}
}
