package collector

import (
	"context"

	"github.com/blink-zero/esxport/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/vmware/govmomi/vim25/mo"
)

var (
	vmNetworkRxBytes = prometheus.NewDesc(
		"esxport_vm_network_rx_bytes",
		"Network bytes received by the VM.",
		vmLabels, nil,
	)
	vmNetworkTxBytes = prometheus.NewDesc(
		"esxport_vm_network_tx_bytes",
		"Network bytes transmitted by the VM.",
		vmLabels, nil,
	)
)

func describeNetworkMetrics(ch chan<- *prometheus.Desc) {
	ch <- vmNetworkRxBytes
	ch <- vmNetworkTxBytes
}

func (c *Collector) collectNetworkMetrics(ctx context.Context, ch chan<- prometheus.Metric, client VSphereClient, target config.Target, vms []mo.VirtualMachine) error {
	stats, err := client.QueryNetworkPerformance(ctx, vms)
	if err != nil {
		return err
	}

	// Build lookup for guest labels by VM name
	vmByName := make(map[string]mo.VirtualMachine, len(vms))
	for _, vm := range vms {
		vmByName[vm.Name] = vm
	}

	for _, stat := range stats {
		if !matchesFilter(stat.VMName, target.Filters.VMInclude, target.Filters.VMExclude) {
			continue
		}

		guestOS, guestIP, esxiHost := "", "", ""
		if vm, ok := vmByName[stat.VMName]; ok {
			guestOS, guestIP, esxiHost = vmGuestLabels(vm)
		}

		ch <- prometheus.MustNewConstMetric(vmNetworkRxBytes, prometheus.GaugeValue,
			float64(stat.RxBytes), stat.VMName, guestOS, guestIP, esxiHost, target.Host)
		ch <- prometheus.MustNewConstMetric(vmNetworkTxBytes, prometheus.GaugeValue,
			float64(stat.TxBytes), stat.VMName, guestOS, guestIP, esxiHost, target.Host)
	}
	return nil
}
