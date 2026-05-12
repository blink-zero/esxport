package collector

import (
	"context"

	"github.com/blink-zero/esxport/internal/config"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	clusterHostCount = prometheus.NewDesc(
		"esxport_cluster_host_count",
		"Number of hosts in the cluster.",
		[]string{"cluster_name", "target"}, nil,
	)
	clusterCPUTotal = prometheus.NewDesc(
		"esxport_cluster_cpu_total_mhz",
		"Total CPU capacity of the cluster in MHz.",
		[]string{"cluster_name", "target"}, nil,
	)
	clusterMemoryTotal = prometheus.NewDesc(
		"esxport_cluster_memory_total_bytes",
		"Total memory capacity of the cluster in bytes.",
		[]string{"cluster_name", "target"}, nil,
	)
)

func describeClusterMetrics(ch chan<- *prometheus.Desc) {
	ch <- clusterHostCount
	ch <- clusterCPUTotal
	ch <- clusterMemoryTotal
}

func (c *Collector) collectClusters(ctx context.Context, ch chan<- prometheus.Metric, client VSphereClient, target config.Target) error {
	clusters, err := client.ClusterComputeResources(ctx)
	if err != nil {
		return err
	}

	for _, cluster := range clusters {
		name := cluster.Name

		ch <- prometheus.MustNewConstMetric(clusterHostCount, prometheus.GaugeValue,
			float64(cluster.Summary.GetComputeResourceSummary().NumHosts), name, target.Host)
		ch <- prometheus.MustNewConstMetric(clusterCPUTotal, prometheus.GaugeValue,
			float64(cluster.Summary.GetComputeResourceSummary().TotalCpu), name, target.Host)
		ch <- prometheus.MustNewConstMetric(clusterMemoryTotal, prometheus.GaugeValue,
			float64(cluster.Summary.GetComputeResourceSummary().TotalMemory), name, target.Host)
	}
	return nil
}
