package collector

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/vmware/govmomi/vim25/types"
)

// collectVMSnapshots emits snapshot count and oldest snapshot age for a VM.
func collectVMSnapshots(ch chan<- prometheus.Metric, vmName, guestOS, guestIP, esxiHost, targetHost string, snapshotInfo *types.VirtualMachineSnapshotInfo) {
	if snapshotInfo == nil || len(snapshotInfo.RootSnapshotList) == 0 {
		ch <- prometheus.MustNewConstMetric(vmSnapshotCount, prometheus.GaugeValue, 0,
			vmName, guestOS, guestIP, esxiHost, targetHost)
		return
	}

	count, oldest := countAndOldestSnapshot(snapshotInfo.RootSnapshotList)
	ch <- prometheus.MustNewConstMetric(vmSnapshotCount, prometheus.GaugeValue, float64(count),
		vmName, guestOS, guestIP, esxiHost, targetHost)

	if !oldest.IsZero() {
		age := time.Since(oldest).Seconds()
		ch <- prometheus.MustNewConstMetric(vmSnapshotAge, prometheus.GaugeValue, age,
			vmName, guestOS, guestIP, esxiHost, targetHost)
	}
}

// countAndOldestSnapshot recursively counts snapshots and finds the oldest creation time.
func countAndOldestSnapshot(snapshots []types.VirtualMachineSnapshotTree) (int, time.Time) {
	count := 0
	var oldest time.Time

	for _, snap := range snapshots {
		count++
		if oldest.IsZero() || snap.CreateTime.Before(oldest) {
			oldest = snap.CreateTime
		}
		childCount, childOldest := countAndOldestSnapshot(snap.ChildSnapshotList)
		count += childCount
		if !childOldest.IsZero() && (oldest.IsZero() || childOldest.Before(oldest)) {
			oldest = childOldest
		}
	}
	return count, oldest
}
