package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"golang.org/x/sync/errgroup"
)

type NetSense struct {
	configPath string
	mu         sync.Mutex
	Config     *Config
	Mqtt       *MqttManager
	Capture    *CaptureManager
	Sensors    []*Sensor
	logger     *slog.Logger
	
	Ctx        context.Context
	Cancel     context.CancelFunc
	managerCancel context.CancelFunc
	sensorCancel context.CancelFunc
}

func NewNetSense(configPath string) (*NetSense, error) {
	config, err := LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	logger := slog.Default()
	ctx, cancel := context.WithCancel(context.Background())
	ns := &NetSense{
		configPath: configPath,
		Config:     config,
		Ctx:        ctx,
		Cancel:     cancel,
		logger:     logger,
	}

	if err := ns.setupManagers(ctx); err != nil {
		cancel()
		return nil, err
	}

	return ns, nil
}

func (ns *NetSense) setupManagers(ctx context.Context) error {
	ns.Mqtt = NewMqttManager(ns.Config.Mqtt, ns.Config.Device, ns.logger)
	
	if len(ns.Config.Sensors) == 0 {
		return errors.New("no sensors configured")
	}
	
	pcapBaseline := ns.Config.Sensors[0].Pcap
	ns.Capture = NewCaptureManager(pcapBaseline, ns.logger)

	ns.Sensors = nil
	for _, sensorCfg := range ns.Config.Sensors {
		ns.Mqtt.RegisterSensor(sensorCfg.Homeassistant)
		sensor := NewSensor(sensorCfg, ns.Mqtt, ns.logger)
		ns.Sensors = append(ns.Sensors, sensor)
		ns.Capture.Register(sensor)
	}

	return nil
}

func (ns *NetSense) Run() error {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	for {
		managerCtx, managerCancel := context.WithCancel(ns.Ctx)
		sensorCtx, sensorCancel := context.WithCancel(managerCtx)
		
		ns.mu.Lock()
		ns.managerCancel = managerCancel
		ns.sensorCancel = sensorCancel
		ns.mu.Unlock()

		g, gCtx := errgroup.WithContext(managerCtx)

		g.Go(func() error {
			return ns.Mqtt.Start(gCtx)
		})
		g.Go(func() error {
			return ns.Capture.Start(gCtx)
		})

		ns.startSensors(sensorCtx)

		// Logger for MQTT messages
		g.Go(func() error {
			for {
				select {
				case <-gCtx.Done():
					return nil
				case message := <-ns.Mqtt.Receive():
					ns.logger.Info("received mqtt publish", "payload", string(message))
				}
			}
		})

		// Wait for signal or context done
		select {
		case <-ns.Ctx.Done():
			managerCancel()
			return g.Wait()
		case sig := <-sigChan:
			switch sig {
			case syscall.SIGHUP:
				ns.logger.Info("reloading configuration")
				managerCancel()
				_ = g.Wait() // Wait for old managers to stop
				if err := ns.Reload(); err != nil {
					ns.logger.Error("reload failed", "error", err)
				}
				continue // Restart loop with new managers
			case os.Interrupt, syscall.SIGTERM:
				ns.logger.Info("shutting down")
				ns.Cancel()
				managerCancel()
				return g.Wait()
			}
		}
	}
}

func (ns *NetSense) startSensors(ctx context.Context) {
	ns.mu.Lock()
	sensors := ns.Sensors
	ns.mu.Unlock()
	for _, s := range sensors {
		s.Start(ctx)
	}
}

func (ns *NetSense) Reload() error {
	ns.mu.Lock()
	configPath := ns.configPath
	ns.mu.Unlock()

	newConfig, err := LoadConfig(configPath)
	if err != nil {
		return err
	}

	ns.mu.Lock()
	ns.Config = newConfig
	ns.mu.Unlock()

	if err := ns.setupManagers(ns.Ctx); err != nil {
		return err
	}

	return nil
}
