package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration.
type Config struct {
	Targets []Target     `yaml:"targets"`
	Server  ServerConfig `yaml:"server"`
	Scrape  ScrapeConfig `yaml:"scrape"`
}

// Target represents a vCenter/ESXi connection target.
type Target struct {
	Host      string        `yaml:"host"`
	Username  string        `yaml:"username"`
	Password  string        `yaml:"password"`
	IgnoreSSL bool          `yaml:"ignore_ssl"`
	Collect   CollectConfig `yaml:"collect"`
	Filters   FilterConfig  `yaml:"filters"`
}

// CollectConfig controls which metric groups to collect.
type CollectConfig struct {
	Hosts          bool `yaml:"hosts"`
	VMs            bool `yaml:"vms"`
	Datastores     bool `yaml:"datastores"`
	Snapshots      bool `yaml:"snapshots"`
	Clusters       bool `yaml:"clusters"`
	NetworkMetrics bool `yaml:"network_metrics"`
	ResourcePools  bool `yaml:"resource_pools"`
}

// FilterConfig controls regex filters for objects.
type FilterConfig struct {
	VMInclude   string `yaml:"vm_include"`
	VMExclude   string `yaml:"vm_exclude"`
	HostInclude string `yaml:"host_include"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port        int    `yaml:"port"`
	MetricsPath string `yaml:"metrics_path"`
}

// ScrapeConfig holds scrape behavior settings.
type ScrapeConfig struct {
	Timeout     string `yaml:"timeout"`
	Concurrency int    `yaml:"concurrency"`
}

// TimeoutDuration parses the timeout string into a time.Duration.
func (s ScrapeConfig) TimeoutDuration() (time.Duration, error) {
	if s.Timeout == "" {
		return 30 * time.Second, nil
	}
	return time.ParseDuration(s.Timeout)
}

// DefaultCollect returns a CollectConfig with everything enabled.
func DefaultCollect() CollectConfig {
	return CollectConfig{
		Hosts:          true,
		VMs:            true,
		Datastores:     true,
		Snapshots:      true,
		Clusters:       true,
		NetworkMetrics: true,
		ResourcePools:  true,
	}
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		Server: ServerConfig{
			Port:        9272,
			MetricsPath: "/metrics",
		},
		Scrape: ScrapeConfig{
			Timeout:     "30s",
			Concurrency: 4,
		},
	}
}

// LoadFile reads and parses a YAML config file, then applies env overrides.
func LoadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}
	return Load(data)
}

// Load parses YAML bytes into a Config, applying defaults and env overrides.
func Load(data []byte) (*Config, error) {
	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config YAML: %w", err)
	}
	applyEnvOverrides(&cfg)
	applyCollectDefaults(&cfg)
	return &cfg, nil
}

// LoadFromEnv creates a config purely from environment variables.
func LoadFromEnv() *Config {
	cfg := DefaultConfig()
	applyEnvOverrides(&cfg)
	applyCollectDefaults(&cfg)
	return &cfg
}

func applyEnvOverrides(cfg *Config) {
	host := os.Getenv("ESXPORT_HOST")
	username := os.Getenv("ESXPORT_USERNAME")
	password := os.Getenv("ESXPORT_PASSWORD")
	portStr := os.Getenv("ESXPORT_PORT")
	ignoreSSL := os.Getenv("ESXPORT_IGNORE_SSL")

	if host != "" {
		// Find or create a target for env-based config
		if len(cfg.Targets) == 0 {
			cfg.Targets = append(cfg.Targets, Target{})
		}
		cfg.Targets[0].Host = host
	}
	if username != "" {
		if len(cfg.Targets) == 0 {
			cfg.Targets = append(cfg.Targets, Target{})
		}
		cfg.Targets[0].Username = username
	}
	if password != "" {
		if len(cfg.Targets) == 0 {
			cfg.Targets = append(cfg.Targets, Target{})
		}
		cfg.Targets[0].Password = password
	}
	if portStr != "" {
		if port, err := strconv.Atoi(portStr); err == nil {
			cfg.Server.Port = port
		}
	}
	if strings.EqualFold(ignoreSSL, "true") || ignoreSSL == "1" {
		if len(cfg.Targets) > 0 {
			cfg.Targets[0].IgnoreSSL = true
		}
	}
}

func applyCollectDefaults(cfg *Config) {
	for i := range cfg.Targets {
		t := &cfg.Targets[i]
		// If all collect fields are false (zero value), enable all
		if !t.Collect.Hosts && !t.Collect.VMs && !t.Collect.Datastores && !t.Collect.Snapshots && !t.Collect.Clusters && !t.Collect.NetworkMetrics && !t.Collect.ResourcePools {
			t.Collect = DefaultCollect()
		}
	}
}
