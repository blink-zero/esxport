package collector

import (
	"context"

	"github.com/blink-zero/esxport/internal/config"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	rpLabels = []string{"pool_name", "pool_id", "target"}

	rpCPUUsage = prometheus.NewDesc(
		"esxport_resourcepool_cpu_usage_mhz",
		"Current CPU usage of the resource pool in MHz.",
		rpLabels, nil,
	)
	rpCPULimit = prometheus.NewDesc(
		"esxport_resourcepool_cpu_limit_mhz",
		"CPU limit of the resource pool in MHz (-1 = unlimited).",
		rpLabels, nil,
	)
	rpMemoryUsage = prometheus.NewDesc(
		"esxport_resourcepool_memory_usage_bytes",
		"Current memory usage of the resource pool in bytes.",
		rpLabels, nil,
	)
	rpMemoryLimit = prometheus.NewDesc(
		"esxport_resourcepool_memory_limit_bytes",
		"Memory limit of the resource pool in bytes (-1 = unlimited).",
		rpLabels, nil,
	)
	rpCPUReservation = prometheus.NewDesc(
		"esxport_resourcepool_cpu_reservation_mhz",
		"CPU reservation of the resource pool in MHz.",
		rpLabels, nil,
	)
	rpMemoryReservation = prometheus.NewDesc(
		"esxport_resourcepool_memory_reservation_bytes",
		"Memory reservation of the resource pool in bytes.",
		rpLabels, nil,
	)
)

func describeResourcePoolMetrics(ch chan<- *prometheus.Desc) {
	ch <- rpCPUUsage
	ch <- rpCPULimit
	ch <- rpCPUReservation
	ch <- rpMemoryUsage
	ch <- rpMemoryLimit
	ch <- rpMemoryReservation
}

func (c *Collector) collectResourcePools(ctx context.Context, ch chan<- prometheus.Metric, client VSphereClient, target config.Target) error {
	pools, err := client.ResourcePools(ctx)
	if err != nil {
		return err
	}

	for _, rp := range pools {
		name := rp.Name

		ch <- prometheus.MustNewConstMetric(rpCPUUsage, prometheus.GaugeValue,
			float64(rp.Runtime.Cpu.OverallUsage), name, rp.Self.Value, target.Host)

		cpuLimit := int64(-1)
		if rp.Config.CpuAllocation.Limit != nil {
			cpuLimit = *rp.Config.CpuAllocation.Limit
		}
		ch <- prometheus.MustNewConstMetric(rpCPULimit, prometheus.GaugeValue,
			float64(cpuLimit), name, rp.Self.Value, target.Host)

		cpuReservation := int64(0)
		if rp.Config.CpuAllocation.Reservation != nil {
			cpuReservation = *rp.Config.CpuAllocation.Reservation
		}
		ch <- prometheus.MustNewConstMetric(rpCPUReservation, prometheus.GaugeValue,
			float64(cpuReservation), name, rp.Self.Value, target.Host)

		ch <- prometheus.MustNewConstMetric(rpMemoryUsage, prometheus.GaugeValue,
			float64(rp.Runtime.Memory.OverallUsage), name, rp.Self.Value, target.Host)

		memLimit := int64(-1)
		if rp.Config.MemoryAllocation.Limit != nil {
			memLimit = *rp.Config.MemoryAllocation.Limit
		}
		ch <- prometheus.MustNewConstMetric(rpMemoryLimit, prometheus.GaugeValue,
			float64(memLimit), name, rp.Self.Value, target.Host)

		memReservation := int64(0)
		if rp.Config.MemoryAllocation.Reservation != nil {
			memReservation = *rp.Config.MemoryAllocation.Reservation
		}
		ch <- prometheus.MustNewConstMetric(rpMemoryReservation, prometheus.GaugeValue,
			float64(memReservation), name, rp.Self.Value, target.Host)
	}
	return nil
}
