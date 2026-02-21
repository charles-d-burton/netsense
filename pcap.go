package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

type CaptureListener interface {
	OnPacket(packet gopacket.Packet, linkType layers.LinkType)
	GetBPFFilter() string
}

type ifaceCapture struct {
	cancel context.CancelFunc
	handle *pcap.Handle
}

type CaptureManager struct {
	mu        sync.RWMutex
	handlers  map[string]*ifaceCapture
	listeners []CaptureListener
	doneChan  chan string
	pcapConfig PcapConfig
	logger     *slog.Logger
}

func NewCaptureManager(pcapConfig PcapConfig, logger *slog.Logger) *CaptureManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &CaptureManager{
		handlers:   make(map[string]*ifaceCapture),
		doneChan:   make(chan string, 10),
		pcapConfig: pcapConfig,
		logger:     logger.With("component", "capture"),
	}
}

func (cm *CaptureManager) Register(listener CaptureListener) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.listeners = append(cm.listeners, listener)
}

func (cm *CaptureManager) Start(ctx context.Context) error {
	go cm.runHotSwap(ctx)
	return nil
}

func (cm *CaptureManager) runHotSwap(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		targetInterfaces, err := cm.getTargetInterfaces()
		if err != nil {
			cm.logger.Error("failed to get target interfaces", "error", err)
		} else {
			cm.updateHandlers(ctx, targetInterfaces)
		}

		select {
		case <-ctx.Done():
			cm.stopAll()
			return
		case <-ticker.C:
		case name := <-cm.doneChan:
			cm.mu.Lock()
			if h, ok := cm.handlers[name]; ok {
				h.cancel()
				delete(cm.handlers, name)
			}
			cm.mu.Unlock()
		}
	}
}

func (cm *CaptureManager) updateHandlers(ctx context.Context, targetInterfaces []net.Interface) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	currentTargets := make(map[string]bool)
	for _, iface := range targetInterfaces {
		currentTargets[iface.Name] = true
		if _, ok := cm.handlers[iface.Name]; !ok {
			cm.logger.Info("starting shared capture", "iface", iface.Name)
			ifaceCtx, ifaceCancel := context.WithCancel(ctx)
			
			var allFilters []string
			for _, l := range cm.listeners {
				if f := l.GetBPFFilter(); f != "" {
					allFilters = append(allFilters, "("+f+")")
				}
			}
			masterFilter := strings.Join(allFilters, " or ")
			
			h, err := pcap.OpenLive(iface.Name, cm.pcapConfig.SnapLen, cm.pcapConfig.Promiscuous, 1*time.Second)
			if err != nil {
				cm.logger.Error("failed to open pcap handle", "iface", iface.Name, "error", err)
				ifaceCancel()
				continue
			}

			if masterFilter != "" {
				if err = h.SetBPFFilter(masterFilter); err != nil {
					cm.logger.Error("failed to set master BPF filter", "iface", iface.Name, "filter", masterFilter, "error", err)
					h.Close()
					ifaceCancel()
					continue
				}
			}

			cm.handlers[iface.Name] = &ifaceCapture{cancel: ifaceCancel, handle: h}
			
			go func(name string, handle *pcap.Handle, c context.Context) {
				defer handle.Close()
				linkType := handle.LinkType()
				packetSource := gopacket.NewPacketSource(handle, linkType)
				packets := packetSource.Packets()
				for {
					select {
					case <-c.Done():
						return
					case packet, ok := <-packets:
						if !ok {
							cm.logger.Error("packet channel closed, stopping capture", "iface", name)
							select {
							case cm.doneChan <- name:
							default:
							}
							return
						}
						cm.mu.RLock()
						for _, l := range cm.listeners {
							l.OnPacket(packet, linkType)
						}
						cm.mu.RUnlock()
					}
				}
			}(iface.Name, h, ifaceCtx)
		}
	}

	for name, h := range cm.handlers {
		if !currentTargets[name] {
			cm.logger.Info("stopping shared capture", "iface", name)
			h.cancel()
			delete(cm.handlers, name)
		}
	}
}

func (cm *CaptureManager) getTargetInterfaces() ([]net.Interface, error) {
	allInterfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to list network interfaces: %w", err)
	}

	var targetInterfaces []net.Interface
	for _, iface := range allInterfaces {
		// Only monitor interfaces that are Up and not Loopback
		if (iface.Flags&net.FlagUp) != 0 && (iface.Flags&net.FlagLoopback) == 0 {
			targetInterfaces = append(targetInterfaces, iface)
		}
	}
	return targetInterfaces, nil
}

func (cm *CaptureManager) stopAll() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	for _, h := range cm.handlers {
		h.cancel()
	}
	cm.handlers = make(map[string]*ifaceCapture)
}

func ValidateBPFFilter(filter string) error {
	if filter == "" {
		return nil
	}
	_, err := pcap.CompileBPFFilter(layers.LinkTypeEthernet, 1, filter)
	return err
}

func BuildBPFFilter(filters []Filter) string {
	var allServiceFilters []string
	for _, f := range filters {
		var serviceConditions []string
		dir := f.Direction
		if dir == "" {
			dir = "dst"
		}

		if len(f.Cidrs) > 0 {
			var cidrParts []string
			for _, cidr := range f.Cidrs {
				switch dir {
				case "src":
					cidrParts = append(cidrParts, "src net "+cidr)
				case "dst":
					cidrParts = append(cidrParts, "dst net "+cidr)
				case "both":
					cidrParts = append(cidrParts, "net "+cidr)
				}
			}
			serviceConditions = append(serviceConditions, "("+strings.Join(cidrParts, " or ")+")")
		}

		if len(f.Portranges) > 0 {
			var portParts []string
			for _, portrange := range f.Portranges {
				switch dir {
				case "src":
					portParts = append(portParts, "src portrange "+portrange)
				case "dst":
					portParts = append(portParts, "dst portrange "+portrange)
				case "both":
					portParts = append(portParts, "portrange "+portrange)
				}
			}
			serviceConditions = append(serviceConditions, "("+strings.Join(portParts, " or ")+")")
		}

		if len(f.Protocols) > 0 {
			var protocolParts []string
			for _, protocol := range f.Protocols {
				protocolParts = append(protocolParts, protocol)
			}
			serviceConditions = append(serviceConditions, "("+strings.Join(protocolParts, " or ")+")")
		}

		if len(serviceConditions) > 0 {
			allServiceFilters = append(allServiceFilters, "("+strings.Join(serviceConditions, " and ")+")")
		}
	}

	if len(allServiceFilters) > 0 {
		return strings.Join(allServiceFilters, " or ")
	}
	return ""
}
