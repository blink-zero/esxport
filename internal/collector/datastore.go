package collector

import (
	"context"

	"github.com/blink-zero/esxport/internal/config"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	datastoreCapacity = prometheus.NewDesc(
		"esxport_datastore_capacity_bytes",
		"Total capacity of the datastore in bytes.",
		[]string{"datastore_name", "datastore_type", "target"}, nil,
	)
	datastoreFree = prometheus.NewDesc(
		"esxport_datastore_free_bytes",
		"Free space on the datastore in bytes.",
		[]string{"datastore_name", "datastore_type", "target"}, nil,
	)
	datastoreUncommitted = prometheus.NewDesc(
		"esxport_datastore_uncommitted_bytes",
		"Uncommitted space on the datastore in bytes.",
		[]string{"datastore_name", "datastore_type", "target"}, nil,
	)
)

func describeDatastoreMetrics(ch chan<- *prometheus.Desc) {
	ch <- datastoreCapacity
	ch <- datastoreFree
	ch <- datastoreUncommitted
}

func (c *Collector) collectDatastores(ctx context.Context, ch chan<- prometheus.Metric, client VSphereClient, target config.Target) error {
	datastores, err := client.Datastores(ctx)
	if err != nil {
		return err
	}

	for _, ds := range datastores {
		name := ds.Name
		dsType := ds.Summary.Type

		ch <- prometheus.MustNewConstMetric(datastoreCapacity, prometheus.GaugeValue,
			float64(ds.Summary.Capacity), name, dsType, target.Host)
		ch <- prometheus.MustNewConstMetric(datastoreFree, prometheus.GaugeValue,
			float64(ds.Summary.FreeSpace), name, dsType, target.Host)
		ch <- prometheus.MustNewConstMetric(datastoreUncommitted, prometheus.GaugeValue,
			float64(ds.Summary.Uncommitted), name, dsType, target.Host)
	}
	return nil
}
