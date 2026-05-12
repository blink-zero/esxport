# esxport

[![CI](https://github.com/blink-zero/esxport/actions/workflows/ci.yml/badge.svg)](https://github.com/blink-zero/esxport/actions/workflows/ci.yml)
[![Release](https://github.com/blink-zero/esxport/actions/workflows/release.yml/badge.svg)](https://github.com/blink-zero/esxport/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/blink-zero/esxport)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

A modern VMware vSphere/ESXi Prometheus exporter written in Go. Built as a replacement for the abandoned [pryorda/vmware_exporter](https://github.com/pryorda/vmware_exporter).

## Why esxport?

| Feature | vmware_exporter | esxport |
|---------|----------------|---------|
| Language | Python | Go |
| Distribution | pip + dependencies | Single binary |
| Docker image | ~200MB+ | ~15MB |
| Startup time | Seconds | Milliseconds |
| Memory usage | High | Low |
| Maintained | Abandoned | Active |
| Multi-target | Limited | Full support |
| Snapshot metrics | Basic | Count + age |
| Cluster metrics | No | Yes |
| Network metrics | Basic | VM rx/tx bytes + host NIC count |
| Resource pool metrics | No | Yes |
| Guest OS labels | No | Yes |
| Health endpoints | No | /healthz + /readyz |
| Connection caching | No | Persistent with auto-reconnect |
| Grafana dashboard | No | Included |

## Quick Start

### Binary

```bash
# Download latest release
curl -sL https://github.com/blink-zero/esxport/releases/latest/download/esxport_linux_amd64.tar.gz | tar xz
chmod +x esxport

# Run with env vars
export ESXPORT_HOST=vcenter.example.com
export ESXPORT_USERNAME=readonly@vsphere.local
export ESXPORT_PASSWORD=secret
./esxport serve

# Or with config file
./esxport serve --config config.yml
```

### Docker

```bash
docker run -d \
  -p 9272:9272 \
  -e ESXPORT_HOST=vcenter.example.com \
  -e ESXPORT_USERNAME=readonly@vsphere.local \
  -e ESXPORT_PASSWORD=secret \
  -e ESXPORT_IGNORE_SSL=true \
  ghcr.io/blink-zero/esxport:latest
```

### Docker Compose (esxport only)

```yaml
# docker-compose.yml
services:
  esxport:
    image: ghcr.io/blink-zero/esxport:latest
    ports:
      - "9272:9272"
    environment:
      - ESXPORT_HOST=vcenter.example.com
      - ESXPORT_USERNAME=readonly@vsphere.local
      - ESXPORT_PASSWORD=secret
      - ESXPORT_IGNORE_SSL=true
    restart: unless-stopped
```

```bash
docker compose up -d
# Metrics at http://localhost:9272/metrics
```

### Docker Compose (full stack -- esxport + Prometheus + Grafana)

```yaml
# docker-compose-full.yml
services:
  esxport:
    image: ghcr.io/blink-zero/esxport:latest
    ports:
      - "9272:9272"
    volumes:
      - ./config.yml:/etc/esxport/config.yml:ro
    command: ["serve", "--config", "/etc/esxport/config.yml"]
    restart: unless-stopped

  prometheus:
    image: prom/prometheus:latest
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - prometheus_data:/prometheus
    depends_on:
      - esxport
    restart: unless-stopped

  grafana:
    image: grafana/grafana:latest
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
    volumes:
      - grafana_data:/var/lib/grafana
    depends_on:
      - prometheus
    restart: unless-stopped

volumes:
  prometheus_data:
  grafana_data:
```

Create `prometheus.yml`:

```yaml
global:
  scrape_interval: 60s

scrape_configs:
  - job_name: esxport
    scrape_timeout: 55s
    static_configs:
      - targets: ["esxport:9272"]
```

```bash
cp config.yml.example config.yml
# Edit config.yml with your vCenter details
docker compose -f docker-compose-full.yml up -d
# Prometheus at http://localhost:9090
# Grafana at http://localhost:3000 (admin/admin)
# Metrics at http://localhost:9272/metrics
```

## CLI Usage

```
esxport serve                    # Start HTTP metrics server (default :9272)
esxport serve --port 9272        # Custom port
esxport serve --config config.yml
esxport check                    # One-shot: connect, collect, print to stdout
esxport version                  # Print version
```

## Health Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /healthz` | Always returns 200 with `{"status":"ok"}`. Use for liveness probes. |
| `GET /readyz` | Returns 200 after at least one successful scrape, 503 otherwise. Use for readiness probes. |

## Configuration

### Config File

```yaml
targets:
  - host: vcenter.example.com
    username: readonly@vsphere.local
    password: secret
    ignore_ssl: true
    collect:
      hosts: true
      vms: true
      datastores: true
      snapshots: true
      clusters: true
      network_metrics: true
      resource_pools: true
    filters:
      vm_include: ".*"
      vm_exclude: "template-.*"
      host_include: ".*"

server:
  port: 9272
  metrics_path: /metrics

scrape:
  timeout: 30s
  concurrency: 4
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `ESXPORT_HOST` | vCenter/ESXi hostname | -- |
| `ESXPORT_USERNAME` | Login username | -- |
| `ESXPORT_PASSWORD` | Login password | -- |
| `ESXPORT_PORT` | HTTP server port | `9272` |
| `ESXPORT_IGNORE_SSL` | Skip TLS verification | `false` |

Environment variables override the first target in the config file.

## Metrics

### Host Metrics

Labels: `host_name`, `target`

| Metric | Type | Description |
|--------|------|-------------|
| `esxport_host_power_state` | gauge | Power state (1=on, 0=other) |
| `esxport_host_cpu_usage_mhz` | gauge | Current CPU usage in MHz |
| `esxport_host_cpu_total_mhz` | gauge | Total CPU capacity in MHz |
| `esxport_host_memory_usage_bytes` | gauge | Current memory usage |
| `esxport_host_memory_total_bytes` | gauge | Total memory capacity |
| `esxport_host_uptime_seconds` | gauge | Host uptime |
| `esxport_host_boot_time` | gauge | Boot time (Unix timestamp) |
| `esxport_host_nic_count` | gauge | Number of physical network adapters |

### VM Metrics

Labels: `vm_name`, `guest_os`, `guest_ip`, `esxi_host`, `target`

> **Breaking change in v0.2.0**: VM metrics now include `guest_os`, `guest_ip`, and `esxi_host` labels. Update your PromQL queries and alert rules accordingly.

| Metric | Type | Description |
|--------|------|-------------|
| `esxport_vm_power_state` | gauge | Power state (1=on, 0=other) |
| `esxport_vm_cpu_usage_mhz` | gauge | CPU usage in MHz |
| `esxport_vm_num_cpu` | gauge | Number of vCPUs |
| `esxport_vm_memory_usage_bytes` | gauge | Guest memory usage |
| `esxport_vm_memory_total_bytes` | gauge | Configured memory |
| `esxport_vm_disk_usage_bytes` | gauge | Committed storage |
| `esxport_vm_uptime_seconds` | gauge | VM uptime |
| `esxport_vm_snapshot_count` | gauge | Number of snapshots |
| `esxport_vm_snapshot_age_seconds` | gauge | Age of oldest snapshot |
| `esxport_vm_tools_status` | gauge | VMware Tools status (0-3) |
| `esxport_vm_template` | gauge | Is template (1/0) |
| `esxport_vm_network_rx_bytes` | gauge | Network bytes received |
| `esxport_vm_network_tx_bytes` | gauge | Network bytes transmitted |

### Datastore Metrics

Labels: `datastore_name`, `datastore_type`, `target`

| Metric | Type | Description |
|--------|------|-------------|
| `esxport_datastore_capacity_bytes` | gauge | Total capacity |
| `esxport_datastore_free_bytes` | gauge | Free space |
| `esxport_datastore_uncommitted_bytes` | gauge | Uncommitted space |

### Cluster Metrics

Labels: `cluster_name`, `target`

| Metric | Type | Description |
|--------|------|-------------|
| `esxport_cluster_host_count` | gauge | Number of hosts |
| `esxport_cluster_cpu_total_mhz` | gauge | Total CPU capacity |
| `esxport_cluster_memory_total_bytes` | gauge | Total memory capacity |

### Resource Pool Metrics

Labels: `pool_name`, `target`

| Metric | Type | Description |
|--------|------|-------------|
| `esxport_resourcepool_cpu_usage_mhz` | gauge | Current CPU usage in MHz |
| `esxport_resourcepool_cpu_limit_mhz` | gauge | CPU limit in MHz (-1 = unlimited) |
| `esxport_resourcepool_memory_usage_bytes` | gauge | Current memory usage in bytes |
| `esxport_resourcepool_memory_limit_bytes` | gauge | Memory limit in bytes (-1 = unlimited) |

### Exporter Meta

Labels: `target`

| Metric | Type | Description |
|--------|------|-------------|
| `esxport_scrape_duration_seconds` | gauge | Scrape duration |
| `esxport_scrape_success` | gauge | Scrape success (1/0) |
| `esxport_scrape_errors_total` | counter | Total scrape errors |

## Prometheus Configuration

```yaml
scrape_configs:
  - job_name: esxport
    scrape_interval: 60s
    scrape_timeout: 55s
    static_configs:
      - targets: ["localhost:9272"]
```

## Grafana Dashboard

A pre-built dashboard is included at [`grafana/dashboard.json`](grafana/dashboard.json). Import it into Grafana via **Dashboards > Import > Upload JSON file**.

The dashboard includes panels for:

- **Host Overview**: CPU/memory utilization %, NIC count, uptime
- **VM Overview**: CPU/memory per VM, power states
- **Network**: VM network rx/tx bytes
- **Datastores**: Capacity vs free space, utilization %
- **Clusters**: Host count, aggregate CPU/memory
- **Resource Pools**: CPU/memory usage and limits
- **Exporter Health**: Scrape success, duration, error counts
- **Snapshot Alerts**: VMs with snapshots, oldest snapshot age

Example PromQL queries:

```promql
# Host CPU utilization %
esxport_host_cpu_usage_mhz / esxport_host_cpu_total_mhz * 100

# Host memory utilization %
esxport_host_memory_usage_bytes / esxport_host_memory_total_bytes * 100

# VMs with snapshots older than 7 days
esxport_vm_snapshot_age_seconds > 604800

# Datastore utilization %
1 - (esxport_datastore_free_bytes / esxport_datastore_capacity_bytes) * 100

# VMs by guest OS
count by (guest_os) (esxport_vm_power_state == 1)

# Resource pool CPU utilization
esxport_resourcepool_cpu_usage_mhz / esxport_resourcepool_cpu_limit_mhz * 100
```

## Building from Source

```bash
git clone https://github.com/blink-zero/esxport.git
cd esxport
make build
```

## Credits

- Inspired by [pryorda/vmware_exporter](https://github.com/pryorda/vmware_exporter) -- the original Python-based VMware Prometheus exporter
- Built with [govmomi](https://github.com/vmware/govmomi) -- VMware's official Go SDK for vSphere
- Metrics exposition via [prometheus/client_golang](https://github.com/prometheus/client_golang)

## License

MIT -- see [LICENSE](LICENSE).
