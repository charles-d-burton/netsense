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
	filter   string

	packetCount atomic.Int64
	bpfs        sync.Map // layers.LinkType -> *pcap.BPF
	bpfMu       sync.Mutex
}

func NewSensor(config SensorConfig, mqtt *MqttManager, logger *slog.Logger) *Sensor {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Sensor{
		Config:   config,
		Mqtt:     mqtt,
		Debounce: NewStateDebouncer(config.Pcap.OnDelay, config.Pcap.OffDelay),
		logger:   logger.With("sensor", config.Name),
	}
	s.filter = BuildBPFFilter(config.Filters)
	return s
}

func (s *Sensor) Start(ctx context.Context) {
	s.logger.Info("starting")
	go s.runLoop(ctx)
}

func (s *Sensor) OnPacket(packet gopacket.Packet, linkType layers.LinkType) {
	if s.filter == "" {
		s.packetCount.Add(1)
		return
	}

	bpfObj, ok := s.bpfs.Load(linkType)
	if !ok {
		s.bpfMu.Lock()
		// Double check
		bpfObj, ok = s.bpfs.Load(linkType)
		if !ok {
			// Compile and store
			bpf, err := pcap.NewBPF(linkType, 65535, s.filter)
			if err != nil {
				s.bpfMu.Unlock()
				s.logger.Error("failed to compile userspace BPF", "linkType", linkType, "filter", s.filter, "error", err)
				return
			}
			s.bpfs.Store(linkType, bpf)
			bpfObj = bpf
		}
		s.bpfMu.Unlock()
	}

	bpf := bpfObj.(*pcap.BPF)

	// Re-verify packet in userspace for accuracy
	data := packet.Data()
	if bpf.Matches(packet.Metadata().CaptureInfo, data) {
		s.packetCount.Add(1)
	}
}

func (s *Sensor) GetBPFFilter() string {
	return s.filter
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
