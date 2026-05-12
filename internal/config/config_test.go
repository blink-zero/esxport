package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadMinimalYAML(t *testing.T) {
	yaml := []byte(`
targets:
  - host: vcenter.example.com
    username: admin
    password: secret
`)
	cfg, err := Load(yaml)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(cfg.Targets))
	}
	if cfg.Targets[0].Host != "vcenter.example.com" {
		t.Errorf("expected host vcenter.example.com, got %s", cfg.Targets[0].Host)
	}
	if cfg.Targets[0].Username != "admin" {
		t.Errorf("expected username admin, got %s", cfg.Targets[0].Username)
	}
}

func TestLoadAppliesDefaults(t *testing.T) {
	yaml := []byte(`
targets:
  - host: vcenter.example.com
    username: admin
    password: secret
`)
	cfg, err := Load(yaml)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port != 9272 {
		t.Errorf("expected default port 9272, got %d", cfg.Server.Port)
	}
	if cfg.Server.MetricsPath != "/metrics" {
		t.Errorf("expected default metrics path /metrics, got %s", cfg.Server.MetricsPath)
	}
	if cfg.Scrape.Concurrency != 4 {
		t.Errorf("expected default concurrency 4, got %d", cfg.Scrape.Concurrency)
	}
	// All collect should be enabled by default
	if !cfg.Targets[0].Collect.Hosts {
		t.Error("expected hosts collection enabled by default")
	}
	if !cfg.Targets[0].Collect.VMs {
		t.Error("expected vms collection enabled by default")
	}
}

func TestLoadFullYAML(t *testing.T) {
	yaml := []byte(`
targets:
  - host: vcenter.example.com
    username: readonly@vsphere.local
    password: secret
    ignore_ssl: true
    collect:
      hosts: true
      vms: true
      datastores: false
      snapshots: true
      clusters: false
    filters:
      vm_include: "prod-.*"
      vm_exclude: "template-.*"
      host_include: "esx-.*"
server:
  port: 9999
  metrics_path: /custom
scrape:
  timeout: 60s
  concurrency: 8
`)
	cfg, err := Load(yaml)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port != 9999 {
		t.Errorf("expected port 9999, got %d", cfg.Server.Port)
	}
	if cfg.Scrape.Concurrency != 8 {
		t.Errorf("expected concurrency 8, got %d", cfg.Scrape.Concurrency)
	}
	target := cfg.Targets[0]
	if !target.IgnoreSSL {
		t.Error("expected ignore_ssl true")
	}
	if target.Filters.VMInclude != "prod-.*" {
		t.Errorf("expected vm_include prod-.*, got %s", target.Filters.VMInclude)
	}
	if !target.Collect.Hosts || !target.Collect.VMs {
		t.Error("expected hosts and vms enabled")
	}
	if target.Collect.Datastores || target.Collect.Clusters {
		t.Error("expected datastores and clusters disabled")
	}
}

func TestEnvOverrides(t *testing.T) {
	t.Setenv("ESXPORT_HOST", "env-host.example.com")
	t.Setenv("ESXPORT_USERNAME", "envuser")
	t.Setenv("ESXPORT_PASSWORD", "envpass")
	t.Setenv("ESXPORT_PORT", "8080")
	t.Setenv("ESXPORT_IGNORE_SSL", "true")

	cfg := LoadFromEnv()
	if len(cfg.Targets) == 0 {
		t.Fatal("expected at least one target from env")
	}
	if cfg.Targets[0].Host != "env-host.example.com" {
		t.Errorf("expected env host, got %s", cfg.Targets[0].Host)
	}
	if cfg.Targets[0].Username != "envuser" {
		t.Errorf("expected env username, got %s", cfg.Targets[0].Username)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}
	if !cfg.Targets[0].IgnoreSSL {
		t.Error("expected ignore_ssl true from env")
	}
}

func TestEnvOverridesYAML(t *testing.T) {
	t.Setenv("ESXPORT_HOST", "override.example.com")
	t.Setenv("ESXPORT_PORT", "7777")

	yaml := []byte(`
targets:
  - host: original.example.com
    username: admin
    password: secret
server:
  port: 9272
`)
	cfg, err := Load(yaml)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Targets[0].Host != "override.example.com" {
		t.Errorf("expected env override host, got %s", cfg.Targets[0].Host)
	}
	if cfg.Server.Port != 7777 {
		t.Errorf("expected env override port 7777, got %d", cfg.Server.Port)
	}
}

func TestTimeoutDuration(t *testing.T) {
	tests := []struct {
		name     string
		timeout  string
		expected time.Duration
	}{
		{"default empty", "", 30 * time.Second},
		{"30 seconds", "30s", 30 * time.Second},
		{"1 minute", "1m", time.Minute},
		{"500ms", "500ms", 500 * time.Millisecond},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := ScrapeConfig{Timeout: tc.timeout}
			d, err := s.TimeoutDuration()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if d != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, d)
			}
		})
	}
}

func TestInvalidYAML(t *testing.T) {
	_, err := Load([]byte(`{{{invalid`))
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestLoadFileNotFound(t *testing.T) {
	_, err := LoadFile("/nonexistent/path/config.yml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadFileValid(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "esxport-config-*.yml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	content := []byte(`
targets:
  - host: test.example.com
    username: user
    password: pass
`)
	if _, err := tmpFile.Write(content); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	cfg, err := LoadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Targets[0].Host != "test.example.com" {
		t.Errorf("expected test.example.com, got %s", cfg.Targets[0].Host)
	}
}
