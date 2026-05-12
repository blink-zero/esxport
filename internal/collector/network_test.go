package collector

import (
	"context"
	"strings"
	"testing"

	"github.com/blink-zero/esxport/internal/config"
	"github.com/blink-zero/esxport/internal/vsphere"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

type mockVSphereClient struct {
	hosts         []mo.HostSystem
	vms           []mo.VirtualMachine
	datastores    []mo.Datastore
	clusters      []mo.ClusterComputeResource
	resourcePools []mo.ResourcePool
	networkStats  []vsphere.NetworkStat
	networkErr    error
}

func (m *mockVSphereClient) HostSystems(_ context.Context) ([]mo.HostSystem, error) {
	return m.hosts, nil
}

func (m *mockVSphereClient) VirtualMachines(_ context.Context) ([]mo.VirtualMachine, error) {
	return m.vms, nil
}

func (m *mockVSphereClient) Datastores(_ context.Context) ([]mo.Datastore, error) {
	return m.datastores, nil
}

func (m *mockVSphereClient) ClusterComputeResources(_ context.Context) ([]mo.ClusterComputeResource, error) {
	return m.clusters, nil
}

func (m *mockVSphereClient) ResourcePools(_ context.Context) ([]mo.ResourcePool, error) {
	return m.resourcePools, nil
}

func (m *mockVSphereClient) QueryNetworkPerformance(_ context.Context, _ []mo.VirtualMachine) ([]vsphere.NetworkStat, error) {
	return m.networkStats, m.networkErr
}

func (m *mockVSphereClient) Close(_ context.Context) error {
	return nil
}

func TestHostNicCount(t *testing.T) {
	client := &mockVSphereClient{
		hosts: []mo.HostSystem{
			{
				ManagedEntity: mo.ManagedEntity{Name: "esxi-01"},
				Summary: types.HostListSummary{
					Hardware: &types.HostHardwareSummary{
						NumNics: 4,
					},
					QuickStats: types.HostListSummaryQuickStats{},
				},
				Runtime: types.HostRuntimeInfo{
					PowerState: "poweredOn",
				},
			},
		},
	}

	target := config.Target{
		Host:    "vcenter.test",
		Collect: config.DefaultCollect(),
	}

	ch := make(chan prometheus.Metric, 100)
	c := &Collector{}
	if err := c.collectHosts(context.Background(), ch, client, target); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	close(ch)

	found := false
	for m := range ch {
		desc := m.Desc().String()
		if strings.Contains(desc, "esxport_host_nic_count") {
			found = true
			pb := readMetric(t, m)
			if pb.GetGauge().GetValue() != 4 {
				t.Errorf("expected nic count 4, got %v", pb.GetGauge().GetValue())
			}
		}
	}
	if !found {
		t.Error("esxport_host_nic_count metric not emitted")
	}
}

func TestCollectNetworkMetrics(t *testing.T) {
	vm := mo.VirtualMachine{
		ManagedEntity: mo.ManagedEntity{Name: "vm-01"},
		Runtime: types.VirtualMachineRuntimeInfo{
			PowerState: "poweredOn",
		},
	}

	client := &mockVSphereClient{
		vms: []mo.VirtualMachine{vm},
		networkStats: []vsphere.NetworkStat{
			vsphere.NetworkStat{VMName: "vm-01", RxBytes: 1024, TxBytes: 2048},
		},
	}

	target := config.Target{
		Host:    "vcenter.test",
		Collect: config.CollectConfig{NetworkMetrics: true, VMs: true},
	}

	ch := make(chan prometheus.Metric, 100)
	c := &Collector{}
	if err := c.collectNetworkMetrics(context.Background(), ch, client, target, client.vms); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	close(ch)

	foundRx := false
	foundTx := false
	for m := range ch {
		desc := m.Desc().String()
		if strings.Contains(desc, "esxport_vm_network_rx_bytes") {
			foundRx = true
			pb := readMetric(t, m)
			if pb.GetGauge().GetValue() != 1024 {
				t.Errorf("expected rx 1024, got %v", pb.GetGauge().GetValue())
			}
		}
		if strings.Contains(desc, "esxport_vm_network_tx_bytes") {
			foundTx = true
			pb := readMetric(t, m)
			if pb.GetGauge().GetValue() != 2048 {
				t.Errorf("expected tx 2048, got %v", pb.GetGauge().GetValue())
			}
		}
	}
	if !foundRx {
		t.Error("esxport_vm_network_rx_bytes metric not emitted")
	}
	if !foundTx {
		t.Error("esxport_vm_network_tx_bytes metric not emitted")
	}
}

func TestCollectNetworkMetricsNoData(t *testing.T) {
	client := &mockVSphereClient{
		vms:          []mo.VirtualMachine{},
		networkStats: nil,
	}

	target := config.Target{
		Host:    "vcenter.test",
		Collect: config.CollectConfig{NetworkMetrics: true},
	}

	ch := make(chan prometheus.Metric, 100)
	c := &Collector{}
	if err := c.collectNetworkMetrics(context.Background(), ch, client, target, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	close(ch)

	count := 0
	for range ch {
		count++
	}
	if count != 0 {
		t.Errorf("expected 0 metrics, got %d", count)
	}
}

func TestVMMetricsHaveGuestLabels(t *testing.T) {
	vms := []mo.VirtualMachine{
		{
			ManagedEntity: mo.ManagedEntity{Name: "vm-01"},
			Summary: types.VirtualMachineSummary{
				Config: types.VirtualMachineConfigSummary{
					NumCpu:       2,
					MemorySizeMB: 4096,
				},
				QuickStats: types.VirtualMachineQuickStats{
					OverallCpuUsage:  500,
					GuestMemoryUsage: 2048,
					UptimeSeconds:    3600,
				},
				Storage: &types.VirtualMachineStorageSummary{
					Committed: 10737418240,
				},
			},
			Runtime: types.VirtualMachineRuntimeInfo{
				PowerState: "poweredOn",
				Host:       &types.ManagedObjectReference{Type: "HostSystem", Value: "host-1"},
			},
			Guest: &types.GuestInfo{
				GuestFullName: "Ubuntu 22.04",
				IpAddress:     "10.0.0.5",
				ToolsStatus:   "toolsOk",
			},
		},
	}

	target := config.Target{
		Host:    "vcenter.test",
		Collect: config.DefaultCollect(),
	}

	ch := make(chan prometheus.Metric, 100)
	c := &Collector{}
	c.emitVMMetrics(ch, vms, target)
	close(ch)

	for m := range ch {
		pb := readMetric(t, m)
		labels := make(map[string]string)
		for _, lp := range pb.GetLabel() {
			labels[lp.GetName()] = lp.GetValue()
		}
		if _, ok := labels["vm_name"]; !ok {
			continue
		}
		if labels["guest_os"] != "Ubuntu 22.04" {
			t.Errorf("metric %s: expected guest_os=Ubuntu 22.04, got %q", m.Desc().String(), labels["guest_os"])
		}
		if labels["guest_ip"] != "10.0.0.5" {
			t.Errorf("metric %s: expected guest_ip=10.0.0.5, got %q", m.Desc().String(), labels["guest_ip"])
		}
		if labels["esxi_host"] != "host-1" {
			t.Errorf("metric %s: expected esxi_host=host-1, got %q", m.Desc().String(), labels["esxi_host"])
		}
	}
}

func TestVMMetricsMissingGuestInfo(t *testing.T) {
	vms := []mo.VirtualMachine{
		{
			ManagedEntity: mo.ManagedEntity{Name: "vm-02"},
			Summary: types.VirtualMachineSummary{
				Config: types.VirtualMachineConfigSummary{
					NumCpu:       1,
					MemorySizeMB: 1024,
				},
				Storage: &types.VirtualMachineStorageSummary{
					Committed: 0,
				},
			},
			Runtime: types.VirtualMachineRuntimeInfo{
				PowerState: "poweredOff",
			},
			Guest: &types.GuestInfo{},
		},
	}

	target := config.Target{
		Host:    "vcenter.test",
		Collect: config.DefaultCollect(),
	}

	ch := make(chan prometheus.Metric, 100)
	c := &Collector{}
	c.emitVMMetrics(ch, vms, target)
	close(ch)

	for m := range ch {
		pb := readMetric(t, m)
		labels := make(map[string]string)
		for _, lp := range pb.GetLabel() {
			labels[lp.GetName()] = lp.GetValue()
		}
		if _, ok := labels["vm_name"]; !ok {
			continue
		}
		if labels["guest_os"] != "" {
			t.Errorf("metric %s: expected empty guest_os, got %q", m.Desc().String(), labels["guest_os"])
		}
		if labels["guest_ip"] != "" {
			t.Errorf("metric %s: expected empty guest_ip, got %q", m.Desc().String(), labels["guest_ip"])
		}
		if labels["esxi_host"] != "" {
			t.Errorf("metric %s: expected empty esxi_host, got %q", m.Desc().String(), labels["esxi_host"])
		}
	}
}

func readMetric(t *testing.T, m prometheus.Metric) *dto.Metric {
	t.Helper()
	pb := &dto.Metric{}
	if err := m.Write(pb); err != nil {
		t.Fatalf("failed to write metric: %v", err)
	}
	return pb
}
