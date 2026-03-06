package main

import (
	"log/slog"
	"os"
)

func main() {
	// Unified Logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	configPath, err := FindConfigFile()
	if err != nil {
		slog.Error("failed to find config file", "error", err)
		os.Exit(1)
	}
	slog.Info("loading config", "path", configPath)

	ns, err := NewNetSense(configPath)
	if err != nil {
		slog.Error("failed to initialize netsense", "error", err)
		os.Exit(1)
	}

	if err := ns.Run(); err != nil {
		slog.Error("netsense error", "error", err)
		os.Exit(1)
	}
}
