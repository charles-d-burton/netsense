package main

import (
	"context"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

func TestSensor_OnPacket(t *testing.T) {
	config := SensorConfig{
		Name: "Test Sensor",
		Filters: []Filter{
			{
				Service:    "test",
				Protocols:  []string{"tcp"},
				Portranges: []string{"80"},
				Direction:  "dst",
			},
		},
		Pcap: PcapConfig{
			ActiveThreshold: 1,
		},
	}
	s := NewSensor(config, nil, slog.Default())

	// Create a TCP packet to port 80
	tcpLayer := &layers.TCP{
		DstPort: 80,
	}
	ipLayer := &layers.IPv4{
		Protocol: layers.IPProtocolTCP,
		SrcIP:    net.IP{192, 168, 1, 1},
		DstIP:    net.IP{192, 168, 1, 2},
	}
	ethLayer := &layers.Ethernet{
		EthernetType: layers.EthernetTypeIPv4,
		SrcMAC:       net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
		DstMAC:       net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x66},
	}
	
	options := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}
	tcpLayer.SetNetworkLayerForChecksum(ipLayer)
	buffer := gopacket.NewSerializeBuffer()
	err := gopacket.SerializeLayers(buffer, options, ethLayer, ipLayer, tcpLayer)
	if err != nil {
		t.Fatal(err)
	}
	data := buffer.Bytes()

	packet := gopacket.NewPacket(data, layers.LayerTypeEthernet, gopacket.Default)
	packet.Metadata().CaptureInfo = gopacket.CaptureInfo{
		CaptureLength: len(data),
		Length:        len(data),
		Timestamp:     time.Now(),
	}

	// First call should compile BPF and match
	s.OnPacket(packet, layers.LinkTypeEthernet)
	if s.packetCount.Load() != 1 {
		t.Errorf("Expected 1 packet, got %d", s.packetCount.Load())
	}

	// Non-matching packet
	tcpLayer.DstPort = 8080
	tcpLayer.SetNetworkLayerForChecksum(ipLayer)
	buffer = gopacket.NewSerializeBuffer()
	_ = gopacket.SerializeLayers(buffer, options, ethLayer, ipLayer, tcpLayer)
	data2 := buffer.Bytes()
	packet2 := gopacket.NewPacket(data2, layers.LayerTypeEthernet, gopacket.Default)
	packet2.Metadata().CaptureInfo = gopacket.CaptureInfo{
		CaptureLength: len(data2),
		Length:        len(data2),
		Timestamp:     time.Now(),
	}
	
	s.OnPacket(packet2, layers.LinkTypeEthernet)
	if s.packetCount.Load() != 1 {
		t.Errorf("Expected still 1 packet after non-matching, got %d", s.packetCount.Load())
	}
}

// Mock test for runLoop requires more effort than justified right now, but
// let's at least test if we can run it and cancel it.
func TestSensor_Lifecycle(t *testing.T) {
	config := SensorConfig{
		Name: "Test Sensor",
		Pcap: PcapConfig{ActiveThreshold: 1},
	}
	s := NewSensor(config, nil, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	s.Start(ctx)
	cancel()
}
