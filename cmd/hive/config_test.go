package main

import (
	"path/filepath"
	"testing"
)

func TestEnsureConfigFileCreatesDefaults(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")

	cfg, err := ensureConfigFile(configPath)
	if err != nil {
		t.Fatalf("ensureConfigFile returned error: %v", err)
	}

	if cfg.Node.Addr != defaultAddr {
		t.Fatalf("expected default addr %q, got %q", defaultAddr, cfg.Node.Addr)
	}
	if cfg.Monitor.Port != defaultMonitorPort {
		t.Fatalf("expected monitor port %d, got %d", defaultMonitorPort, cfg.Monitor.Port)
	}
	if cfg.Discovery.Enabled == nil || *cfg.Discovery.Enabled != defaultAutoDiscovery {
		t.Fatalf("expected discovery enabled %v, got %v", defaultAutoDiscovery, cfg.Discovery.Enabled)
	}
	if cfg.Discovery.Port != defaultDiscoveryPort {
		t.Fatalf("expected discovery port %d, got %d", defaultDiscoveryPort, cfg.Discovery.Port)
	}
	if cfg.Storage.MaxBytes != defaultMaxStorage {
		t.Fatalf("expected max bytes %d, got %d", defaultMaxStorage, cfg.Storage.MaxBytes)
	}

	loaded, err := loadConfigFile(configPath)
	if err != nil {
		t.Fatalf("loadConfigFile returned error: %v", err)
	}
	if loaded.Storage.MaxBytes != defaultMaxStorage {
		t.Fatalf("expected persisted max bytes %d, got %d", defaultMaxStorage, loaded.Storage.MaxBytes)
	}
}

func TestResolveRuntimeConfigUsesFileValuesWhenFlagsAbsent(t *testing.T) {
	enabled := false
	fileCfg := fileConfig{}
	fileCfg.Node.Addr = "0.0.0.0:9000"
	fileCfg.Node.Bootstrap = "10.0.0.2:8000"
	fileCfg.Monitor.Port = 9001
	fileCfg.Discovery.Enabled = &enabled
	fileCfg.Discovery.Port = 9002
	fileCfg.Storage.MaxBytes = 12345

	resolved := resolveRuntimeConfig(fileCfg, cliConfig{
		Addr:          defaultAddr,
		Bootstrap:     defaultBootstrap,
		MonitorPort:   defaultMonitorPort,
		AutoDiscovery: defaultAutoDiscovery,
		DiscoveryPort: defaultDiscoveryPort,
		MaxStorage:    defaultMaxStorage,
	}, map[string]bool{})

	if resolved.Addr != fileCfg.Node.Addr ||
		resolved.Bootstrap != fileCfg.Node.Bootstrap ||
		resolved.MonitorPort != fileCfg.Monitor.Port ||
		resolved.AutoDiscovery != enabled ||
		resolved.DiscoveryPort != fileCfg.Discovery.Port ||
		resolved.MaxStorage != fileCfg.Storage.MaxBytes {
		t.Fatalf("resolved config did not honor file values: %+v", resolved)
	}
}

func TestResolveRuntimeConfigFlagsOverrideFileValues(t *testing.T) {
	enabled := false
	fileCfg := fileConfig{}
	fileCfg.Node.Addr = "0.0.0.0:9000"
	fileCfg.Node.Bootstrap = "10.0.0.2:8000"
	fileCfg.Monitor.Port = 9001
	fileCfg.Discovery.Enabled = &enabled
	fileCfg.Discovery.Port = 9002
	fileCfg.Storage.MaxBytes = 12345

	resolved := resolveRuntimeConfig(fileCfg, cliConfig{
		Addr:          "127.0.0.1:7777",
		Bootstrap:     "127.0.0.1:8888",
		MonitorPort:   7778,
		AutoDiscovery: true,
		DiscoveryPort: 7779,
		MaxStorage:    54321,
	}, map[string]bool{
		"addr":           true,
		"bootstrap":      true,
		"monitor-port":   true,
		"auto-discovery": true,
		"discovery-port": true,
		"max-storage":    true,
	})

	if resolved.Addr != "127.0.0.1:7777" ||
		resolved.Bootstrap != "127.0.0.1:8888" ||
		resolved.MonitorPort != 7778 ||
		resolved.AutoDiscovery != true ||
		resolved.DiscoveryPort != 7779 ||
		resolved.MaxStorage != 54321 {
		t.Fatalf("resolved config did not honor flag overrides: %+v", resolved)
	}
}
