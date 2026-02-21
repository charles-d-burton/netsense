package main

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

type Sensor struct {
	Config   SensorConfig
	Mqtt     *MqttManager
	Debounce *StateDebouncer
	logger   *slog.Logger
	
	packetCount atomic.Int64
	bpfs        sync.Map // layers.LinkType -> *pcap.BPF
}

func NewSensor(config SensorConfig, mqtt *MqttManager, logger *slog.Logger) *Sensor {
	if logger == nil {
		logger = slog.Default()
	}
	return &Sensor{
		Config:   config,
		Mqtt:     mqtt,
		Debounce: NewStateDebouncer(config.Pcap.OnDelay, config.Pcap.OffDelay),
		logger:   logger.With("sensor", config.Name),
	}
}

func (s *Sensor) Start(ctx context.Context) {
	s.logger.Info("starting")
	go s.runLoop(ctx)
}

func (s *Sensor) OnPacket(packet gopacket.Packet, linkType layers.LinkType) {
	filter := s.GetBPFFilter()
	if filter == "" {
		s.packetCount.Add(1)
		return
	}

	bpfObj, ok := s.bpfs.Load(linkType)
	if !ok {
		// Compile and store
		bpf, err := pcap.NewBPF(linkType, 65535, filter)
		if err != nil {
			s.logger.Error("failed to compile userspace BPF", "linkType", linkType, "filter", filter, "error", err)
			return
		}
		s.bpfs.Store(linkType, bpf)
		bpfObj = bpf
	}

	bpf := bpfObj.(*pcap.BPF)

	// Re-verify packet in userspace for accuracy
	data := packet.Data()
	if bpf.Matches(packet.Metadata().CaptureInfo, data) {
		s.packetCount.Add(1)
	}
}

func (s *Sensor) GetBPFFilter() string {
	return BuildBPFFilter(s.Config.Filters)
}

func (s *Sensor) runLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("stopping")
			return
		case <-ticker.C:
			pcapCount := s.packetCount.Swap(0)
			active := pcapCount >= int64(s.Config.Pcap.ActiveThreshold)
			
			changed, state := s.Debounce.Update(active)
			if changed {
				s.logger.Info("state change", "state", state, "packets", pcapCount)
				s.Mqtt.PublishState(s.Config.Homeassistant, state)
			}
		}
	}
}
