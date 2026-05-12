package collector

import (
	"context"
	"log/slog"
	"regexp"
	"sync"
	"time"

	"github.com/blink-zero/esxport/internal/config"
	"github.com/blink-zero/esxport/internal/vsphere"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/vmware/govmomi/vim25/mo"
)

// VSphereClient defines the interface for vSphere data retrieval.
// This enables testing with mocks.
type VSphereClient interface {
	HostSystems(ctx context.Context) ([]mo.HostSystem, error)
	VirtualMachines(ctx context.Context) ([]mo.VirtualMachine, error)
	Datastores(ctx context.Context) ([]mo.Datastore, error)
	ClusterComputeResources(ctx context.Context) ([]mo.ClusterComputeResource, error)
	ResourcePools(ctx context.Context) ([]mo.ResourcePool, error)
	QueryNetworkPerformance(ctx context.Context, vms []mo.VirtualMachine) ([]vsphere.NetworkStat, error)
	Close(ctx context.Context) error
}

// Verify vsphere.Client implements VSphereClient.
var _ VSphereClient = (*vsphere.Client)(nil)

// scrapeMetrics are exporter meta-metrics.
var (
	scrapeDuration = prometheus.NewDesc(
		"esxport_scrape_duration_seconds",
		"Duration of the last scrape in seconds.",
		[]string{"target"}, nil,
	)
	scrapeSuccess = prometheus.NewDesc(
		"esxport_scrape_success",
		"Whether the last scrape was successful (1=success, 0=failure).",
		[]string{"target"}, nil,
	)
	scrapeErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "esxport_scrape_errors_total",
			Help: "Total number of scrape errors.",
		},
		[]string{"target"},
	)
)

func init() {
	prometheus.MustRegister(scrapeErrorsTotal)
}

// OnScrapeComplete is called after each target scrape with the target host and success status.
type OnScrapeComplete func(target string, success bool)

// Collector implements prometheus.Collector for vSphere metrics.
type Collector struct {
	targets          []config.Target
	logger           *slog.Logger
	timeout          time.Duration
	pool             *vsphere.Pool
	onScrapeComplete OnScrapeComplete
}

// New creates a new Collector.
func New(targets []config.Target, timeout time.Duration, logger *slog.Logger) *Collector {
	return &Collector{
		targets: targets,
		logger:  logger,
		timeout: timeout,
	}
}

// SetPool sets a connection pool for caching vSphere connections.
func (c *Collector) SetPool(pool *vsphere.Pool) {
	c.pool = pool
}

// SetOnScrapeComplete sets a callback invoked after each target scrape.
func (c *Collector) SetOnScrapeComplete(fn OnScrapeComplete) {
	c.onScrapeComplete = fn
}

// Describe implements prometheus.Collector.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- scrapeDuration
	ch <- scrapeSuccess
	describeHostMetrics(ch)
	describeVMMetrics(ch)
	describeNetworkMetrics(ch)
	describeDatastoreMetrics(ch)
	describeClusterMetrics(ch)
	describeResourcePoolMetrics(ch)
}

// Collect implements prometheus.Collector.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	var wg sync.WaitGroup
	for _, target := range c.targets {
		wg.Add(1)
		go func(t config.Target) {
			defer wg.Done()
			c.collectTarget(ch, t)
		}(target)
	}
	wg.Wait()
}

func (c *Collector) collectTarget(ch chan<- prometheus.Metric, target config.Target) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	cfg := vsphere.ConnectConfig{
		Host:      target.Host,
		Username:  target.Username,
		Password:  target.Password,
		IgnoreSSL: target.IgnoreSSL,
	}

	var client *vsphere.Client
	var err error
	closeAfter := false

	if c.pool != nil {
		client, err = c.pool.Get(ctx, cfg)
	} else {
		client, err = vsphere.Connect(ctx, cfg)
		closeAfter = true
	}

	if err != nil {
		c.logger.Error("failed to connect to vSphere", "target", target.Host, "error", err)
		ch <- prometheus.MustNewConstMetric(scrapeSuccess, prometheus.GaugeValue, 0, target.Host)
		ch <- prometheus.MustNewConstMetric(scrapeDuration, prometheus.GaugeValue, time.Since(start).Seconds(), target.Host)
		scrapeErrorsTotal.WithLabelValues(target.Host).Inc()
		if c.onScrapeComplete != nil {
			c.onScrapeComplete(target.Host, false)
		}
		return
	}
	if closeAfter {
		defer client.Close(ctx)
	}

	c.collectAll(ctx, ch, client, target)

	ch <- prometheus.MustNewConstMetric(scrapeSuccess, prometheus.GaugeValue, 1, target.Host)
	ch <- prometheus.MustNewConstMetric(scrapeDuration, prometheus.GaugeValue, time.Since(start).Seconds(), target.Host)
	if c.onScrapeComplete != nil {
		c.onScrapeComplete(target.Host, true)
	}
}

func (c *Collector) collectAll(ctx context.Context, ch chan<- prometheus.Metric, client VSphereClient, target config.Target) {
	var wg sync.WaitGroup

	if target.Collect.Hosts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := c.collectHosts(ctx, ch, client, target); err != nil {
				c.logger.Error("failed to collect host metrics", "target", target.Host, "error", err)
				scrapeErrorsTotal.WithLabelValues(target.Host).Inc()
			}
		}()
	}

	if target.Collect.VMs || target.Collect.NetworkMetrics {
		wg.Add(1)
		go func() {
			defer wg.Done()
			vms, err := client.VirtualMachines(ctx)
			if err != nil {
				c.logger.Error("failed to retrieve VMs", "target", target.Host, "error", err)
				scrapeErrorsTotal.WithLabelValues(target.Host).Inc()
				return
			}

			var vmWg sync.WaitGroup

			if target.Collect.VMs {
				vmWg.Add(1)
				go func() {
					defer vmWg.Done()
					c.emitVMMetrics(ch, vms, target)
				}()
			}

			if target.Collect.NetworkMetrics {
				vmWg.Add(1)
				go func() {
					defer vmWg.Done()
					if err := c.collectNetworkMetrics(ctx, ch, client, target, vms); err != nil {
						c.logger.Error("failed to collect network metrics", "target", target.Host, "error", err)
						scrapeErrorsTotal.WithLabelValues(target.Host).Inc()
					}
				}()
			}

			vmWg.Wait()
		}()
	}

	if target.Collect.Datastores {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := c.collectDatastores(ctx, ch, client, target); err != nil {
				c.logger.Error("failed to collect datastore metrics", "target", target.Host, "error", err)
				scrapeErrorsTotal.WithLabelValues(target.Host).Inc()
			}
		}()
	}

	if target.Collect.Clusters {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := c.collectClusters(ctx, ch, client, target); err != nil {
				c.logger.Error("failed to collect cluster metrics", "target", target.Host, "error", err)
				scrapeErrorsTotal.WithLabelValues(target.Host).Inc()
			}
		}()
	}

	if target.Collect.ResourcePools {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := c.collectResourcePools(ctx, ch, client, target); err != nil {
				c.logger.Error("failed to collect resource pool metrics", "target", target.Host, "error", err)
				scrapeErrorsTotal.WithLabelValues(target.Host).Inc()
			}
		}()
	}

	wg.Wait()
}

// matchesFilter checks if a name matches include/exclude regex filters.
func matchesFilter(name, include, exclude string) bool {
	if include != "" {
		matched, err := regexp.MatchString(include, name)
		if err != nil || !matched {
			return false
		}
	}
	if exclude != "" {
		matched, err := regexp.MatchString(exclude, name)
		if err == nil && matched {
			return false
		}
	}
	return true
}
