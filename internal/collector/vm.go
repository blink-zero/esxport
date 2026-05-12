package collector

import (
	"github.com/blink-zero/esxport/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/vmware/govmomi/vim25/mo"
)

// vmLabels is the standard label set for all VM metrics.
var vmLabels = []string{"vm_name", "guest_os", "guest_ip", "esxi_host", "target"}

var (
	vmPowerState = prometheus.NewDesc(
		"esxport_vm_power_state",
		"Power state of the VM (1=poweredOn, 0=other).",
		vmLabels, nil,
	)
	vmCPUUsage = prometheus.NewDesc(
		"esxport_vm_cpu_usage_mhz",
		"Current CPU usage in MHz.",
		vmLabels, nil,
	)
	vmNumCPU = prometheus.NewDesc(
		"esxport_vm_num_cpu",
		"Number of virtual CPUs.",
		vmLabels, nil,
	)
	vmMemoryUsage = prometheus.NewDesc(
		"esxport_vm_memory_usage_bytes",
		"Current guest memory usage in bytes.",
		vmLabels, nil,
	)
	vmMemoryTotal = prometheus.NewDesc(
		"esxport_vm_memory_total_bytes",
		"Total configured memory in bytes.",
		vmLabels, nil,
	)
	vmDiskUsage = prometheus.NewDesc(
		"esxport_vm_disk_usage_bytes",
		"Committed storage in bytes.",
		vmLabels, nil,
	)
	vmUptimeSeconds = prometheus.NewDesc(
		"esxport_vm_uptime_seconds",
		"VM uptime in seconds.",
		vmLabels, nil,
	)
	vmSnapshotCount = prometheus.NewDesc(
		"esxport_vm_snapshot_count",
		"Number of snapshots.",
		vmLabels, nil,
	)
	vmSnapshotAge = prometheus.NewDesc(
		"esxport_vm_snapshot_age_seconds",
		"Age of the oldest snapshot in seconds.",
		vmLabels, nil,
	)
	vmToolsStatus = prometheus.NewDesc(
		"esxport_vm_tools_status",
		"VMware Tools status (0=notInstalled, 1=notRunning, 2=running, 3=runningOld).",
		[]string{"vm_name", "tools_status", "guest_os", "guest_ip", "esxi_host", "target"}, nil,
	)
	vmTemplate = prometheus.NewDesc(
		"esxport_vm_template",
		"Whether the VM is a template (1=template, 0=vm).",
		vmLabels, nil,
	)
)

func describeVMMetrics(ch chan<- *prometheus.Desc) {
	ch <- vmPowerState
	ch <- vmCPUUsage
	ch <- vmNumCPU
	ch <- vmMemoryUsage
	ch <- vmMemoryTotal
	ch <- vmDiskUsage
	ch <- vmUptimeSeconds
	ch <- vmSnapshotCount
	ch <- vmSnapshotAge
	ch <- vmToolsStatus
	ch <- vmTemplate
}

// toolsStatusToFloat encodes VMware Tools status as a numeric value.
func toolsStatusToFloat(status string) float64 {
	switch status {
	case "toolsNotInstalled":
		return 0
	case "toolsNotRunning":
		return 1
	case "toolsOk":
		return 2
	case "toolsOld":
		return 3
	default:
		return -1
	}
}

// vmGuestLabels extracts guest OS labels from a VM.
func vmGuestLabels(vm mo.VirtualMachine) (guestOS, guestIP, esxiHost string) {
	if vm.Guest != nil {
		guestOS = vm.Guest.GuestFullName
		guestIP = vm.Guest.IpAddress
	}
	if vm.Runtime.Host != nil {
		esxiHost = vm.Runtime.Host.Value
	}
	return
}

func (c *Collector) emitVMMetrics(ch chan<- prometheus.Metric, vms []mo.VirtualMachine, target config.Target) {
	for _, vm := range vms {
		name := vm.Name
		if !matchesFilter(name, target.Filters.VMInclude, target.Filters.VMExclude) {
			continue
		}

		guestOS, guestIP, esxiHost := vmGuestLabels(vm)

		// Power state
		powerVal := 0.0
		if vm.Runtime.PowerState == "poweredOn" {
			powerVal = 1.0
		}
		ch <- prometheus.MustNewConstMetric(vmPowerState, prometheus.GaugeValue, powerVal,
			name, guestOS, guestIP, esxiHost, target.Host)

		// CPU usage
		ch <- prometheus.MustNewConstMetric(vmCPUUsage, prometheus.GaugeValue,
			float64(vm.Summary.QuickStats.OverallCpuUsage),
			name, guestOS, guestIP, esxiHost, target.Host)

		// Num CPU
		if vm.Summary.Config.NumCpu > 0 {
			ch <- prometheus.MustNewConstMetric(vmNumCPU, prometheus.GaugeValue,
				float64(vm.Summary.Config.NumCpu),
				name, guestOS, guestIP, esxiHost, target.Host)
		}

		// Memory usage (QuickStats reports in MB)
		ch <- prometheus.MustNewConstMetric(vmMemoryUsage, prometheus.GaugeValue,
			float64(vm.Summary.QuickStats.GuestMemoryUsage)*1024*1024,
			name, guestOS, guestIP, esxiHost, target.Host)

		// Memory total (config in MB)
		ch <- prometheus.MustNewConstMetric(vmMemoryTotal, prometheus.GaugeValue,
			float64(vm.Summary.Config.MemorySizeMB)*1024*1024,
			name, guestOS, guestIP, esxiHost, target.Host)

		// Disk usage
		ch <- prometheus.MustNewConstMetric(vmDiskUsage, prometheus.GaugeValue,
			float64(vm.Summary.Storage.Committed),
			name, guestOS, guestIP, esxiHost, target.Host)

		// Uptime
		if vm.Summary.QuickStats.UptimeSeconds > 0 {
			ch <- prometheus.MustNewConstMetric(vmUptimeSeconds, prometheus.GaugeValue,
				float64(vm.Summary.QuickStats.UptimeSeconds),
				name, guestOS, guestIP, esxiHost, target.Host)
		}

		// Snapshots
		collectVMSnapshots(ch, name, guestOS, guestIP, esxiHost, target.Host, vm.Snapshot)

		// Tools status
		toolsStatus := ""
		if vm.Guest != nil {
			toolsStatus = string(vm.Guest.ToolsStatus)
		}
		ch <- prometheus.MustNewConstMetric(vmToolsStatus, prometheus.GaugeValue,
			toolsStatusToFloat(toolsStatus),
			name, toolsStatus, guestOS, guestIP, esxiHost, target.Host)

		// Template
		templateVal := 0.0
		if vm.Summary.Config.Template {
			templateVal = 1.0
		}
		ch <- prometheus.MustNewConstMetric(vmTemplate, prometheus.GaugeValue, templateVal,
			name, guestOS, guestIP, esxiHost, target.Host)
	}
}
