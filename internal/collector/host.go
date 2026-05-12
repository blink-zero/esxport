package collector

import (
	"context"

	"github.com/blink-zero/esxport/internal/config"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	hostPowerState = prometheus.NewDesc(
		"esxport_host_power_state",
		"Power state of the ESXi host (1=poweredOn, 0=other).",
		[]string{"host_name", "target"}, nil,
	)
	hostCPUUsage = prometheus.NewDesc(
		"esxport_host_cpu_usage_mhz",
		"Current CPU usage in MHz.",
		[]string{"host_name", "target"}, nil,
	)
	hostCPUTotal = prometheus.NewDesc(
		"esxport_host_cpu_total_mhz",
		"Total CPU capacity in MHz.",
		[]string{"host_name", "target"}, nil,
	)
	hostMemoryUsage = prometheus.NewDesc(
		"esxport_host_memory_usage_bytes",
		"Current memory usage in bytes.",
		[]string{"host_name", "target"}, nil,
	)
	hostMemoryTotal = prometheus.NewDesc(
		"esxport_host_memory_total_bytes",
		"Total memory capacity in bytes.",
		[]string{"host_name", "target"}, nil,
	)
	hostUptime = prometheus.NewDesc(
		"esxport_host_uptime_seconds",
		"Uptime of the host in seconds.",
		[]string{"host_name", "target"}, nil,
	)
	hostBootTime = prometheus.NewDesc(
		"esxport_host_boot_time",
		"Boot time of the host as Unix timestamp.",
		[]string{"host_name", "target"}, nil,
	)
	hostNicCount = prometheus.NewDesc(
		"esxport_host_nic_count",
		"Number of physical network adapters on the host.",
		[]string{"host_name", "target"}, nil,
	)
)

func describeHostMetrics(ch chan<- *prometheus.Desc) {
	ch <- hostPowerState
	ch <- hostCPUUsage
	ch <- hostCPUTotal
	ch <- hostMemoryUsage
	ch <- hostMemoryTotal
	ch <- hostUptime
	ch <- hostBootTime
	ch <- hostNicCount
}

func (c *Collector) collectHosts(ctx context.Context, ch chan<- prometheus.Metric, client VSphereClient, target config.Target) error {
	hosts, err := client.HostSystems(ctx)
	if err != nil {
		return err
	}

	for _, host := range hosts {
		name := host.Name
		if !matchesFilter(name, target.Filters.HostInclude, "") {
			continue
		}

		// Power state
		powerVal := 0.0
		if host.Runtime.PowerState == "poweredOn" {
			powerVal = 1.0
		}
		ch <- prometheus.MustNewConstMetric(hostPowerState, prometheus.GaugeValue, powerVal, name, target.Host)

		if host.Summary.QuickStats.OverallCpuUsage > 0 || powerVal == 1 {
			ch <- prometheus.MustNewConstMetric(hostCPUUsage, prometheus.GaugeValue,
				float64(host.Summary.QuickStats.OverallCpuUsage), name, target.Host)
		}

		if host.Summary.Hardware != nil {
			totalMHz := int64(host.Summary.Hardware.CpuMhz) * int64(host.Summary.Hardware.NumCpuCores)
			ch <- prometheus.MustNewConstMetric(hostCPUTotal, prometheus.GaugeValue, float64(totalMHz), name, target.Host)
			ch <- prometheus.MustNewConstMetric(hostMemoryTotal, prometheus.GaugeValue,
				float64(host.Summary.Hardware.MemorySize), name, target.Host)
			ch <- prometheus.MustNewConstMetric(hostNicCount, prometheus.GaugeValue,
				float64(host.Summary.Hardware.NumNics), name, target.Host)
		}

		// Memory usage (QuickStats reports in MB)
		ch <- prometheus.MustNewConstMetric(hostMemoryUsage, prometheus.GaugeValue,
			float64(host.Summary.QuickStats.OverallMemoryUsage)*1024*1024, name, target.Host)

		// Uptime
		if host.Summary.QuickStats.Uptime > 0 {
			ch <- prometheus.MustNewConstMetric(hostUptime, prometheus.GaugeValue,
				float64(host.Summary.QuickStats.Uptime), name, target.Host)
		}

		// Boot time
		if host.Runtime.BootTime != nil {
			ch <- prometheus.MustNewConstMetric(hostBootTime, prometheus.GaugeValue,
				float64(host.Runtime.BootTime.Unix()), name, target.Host)
		}
	}
	return nil
}
