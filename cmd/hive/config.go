package main

import (
	"encoding/json"
	"os"
)

const (
	defaultAddr          = "127.0.0.1:0"
	defaultBootstrap     = ""
	defaultMonitorPort   = 7439
	defaultAutoDiscovery = true
	defaultDiscoveryPort = 7441
	defaultMaxStorage    = 1024 * 1024 * 1024
)

type fileConfig struct {
	Node struct {
		Addr      string `json:"addr,omitempty"`
		Bootstrap string `json:"bootstrap,omitempty"`
	} `json:"node,omitempty"`
	Discovery struct {
		Enabled *bool `json:"enabled,omitempty"`
		Port    int   `json:"port,omitempty"`
	} `json:"discovery,omitempty"`
	Monitor struct {
		Port int `json:"port,omitempty"`
	} `json:"monitor,omitempty"`
	Storage struct {
		MaxBytes int64 `json:"max_bytes,omitempty"`
	} `json:"storage,omitempty"`
}

type runtimeConfig struct {
	Addr          string
	Bootstrap     string
	MonitorPort   int
	AutoDiscovery bool
	DiscoveryPort int
	MaxStorage    int64
}

type cliConfig struct {
	Addr          string
	Bootstrap     string
	MonitorPort   int
	AutoDiscovery bool
	DiscoveryPort int
	MaxStorage    int64
}

func defaultFileConfig() fileConfig {
	cfg := fileConfig{}
	cfg.Node.Addr = defaultAddr
	cfg.Node.Bootstrap = defaultBootstrap
	cfg.Monitor.Port = defaultMonitorPort
	enabled := defaultAutoDiscovery
	cfg.Discovery.Enabled = &enabled
	cfg.Discovery.Port = defaultDiscoveryPort
	cfg.Storage.MaxBytes = defaultMaxStorage
	return cfg
}

func ensureConfigFile(path string) (fileConfig, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		cfg := defaultFileConfig()
		if err := writeConfigFile(path, cfg); err != nil {
			return fileConfig{}, err
		}
		return cfg, nil
	}
	return loadConfigFile(path)
}

func loadConfigFile(path string) (fileConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return fileConfig{}, err
	}
	var cfg fileConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fileConfig{}, err
	}
	return cfg, nil
}

func writeConfigFile(path string, cfg fileConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0600)
}

func resolveRuntimeConfig(fileCfg fileConfig, cliCfg cliConfig, explicitFlags map[string]bool) runtimeConfig {
	cfg := runtimeConfig{
		Addr:          defaultAddr,
		Bootstrap:     defaultBootstrap,
		MonitorPort:   defaultMonitorPort,
		AutoDiscovery: defaultAutoDiscovery,
		DiscoveryPort: defaultDiscoveryPort,
		MaxStorage:    defaultMaxStorage,
	}

	if fileCfg.Node.Addr != "" {
		cfg.Addr = fileCfg.Node.Addr
	}
	cfg.Bootstrap = fileCfg.Node.Bootstrap
	if fileCfg.Monitor.Port > 0 {
		cfg.MonitorPort = fileCfg.Monitor.Port
	}
	if fileCfg.Discovery.Enabled != nil {
		cfg.AutoDiscovery = *fileCfg.Discovery.Enabled
	}
	if fileCfg.Discovery.Port > 0 {
		cfg.DiscoveryPort = fileCfg.Discovery.Port
	}
	if fileCfg.Storage.MaxBytes > 0 {
		cfg.MaxStorage = fileCfg.Storage.MaxBytes
	}

	if explicitFlags["addr"] {
		cfg.Addr = cliCfg.Addr
	}
	if explicitFlags["bootstrap"] {
		cfg.Bootstrap = cliCfg.Bootstrap
	}
	if explicitFlags["monitor-port"] {
		cfg.MonitorPort = cliCfg.MonitorPort
	}
	if explicitFlags["auto-discovery"] {
		cfg.AutoDiscovery = cliCfg.AutoDiscovery
	}
	if explicitFlags["discovery-port"] {
		cfg.DiscoveryPort = cliCfg.DiscoveryPort
	}
	if explicitFlags["max-storage"] {
		cfg.MaxStorage = cliCfg.MaxStorage
	}

	return cfg
}
