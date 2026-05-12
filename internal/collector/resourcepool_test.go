package collector

import (
	"context"
	"strings"
	"testing"

	"github.com/blink-zero/esxport/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

func TestCollectResourcePools(t *testing.T) {
	limit := int64(8000)
	memLimit := int64(17179869184)
	cpuRes := int64(1000)
	memRes := int64(4294967296)
	pools := []mo.ResourcePool{
		{
			ManagedEntity: mo.ManagedEntity{ExtensibleManagedObject: mo.ExtensibleManagedObject{Self: types.ManagedObjectReference{Type: "ResourcePool", Value: "resgroup-123"}}, Name: "production"},
			Runtime: types.ResourcePoolRuntimeInfo{
				Cpu:    types.ResourcePoolResourceUsage{OverallUsage: 2400},
				Memory: types.ResourcePoolResourceUsage{OverallUsage: 8589934592},
			},
			Config: types.ResourceConfigSpec{
				CpuAllocation:    types.ResourceAllocationInfo{Limit: &limit, Reservation: &cpuRes},
				MemoryAllocation: types.ResourceAllocationInfo{Limit: &memLimit, Reservation: &memRes},
			},
		},
	}

	client := &mockVSphereClient{
		resourcePools: pools,
	}

	target := config.Target{
		Host:    "vcenter.test",
		Collect: config.CollectConfig{ResourcePools: true},
	}

	ch := make(chan prometheus.Metric, 100)
	c := &Collector{}
	if err := c.collectResourcePools(context.Background(), ch, client, target); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	close(ch)

	found := map[string]float64{}
	for m := range ch {
		desc := m.Desc().String()
		pb := readMetric(t, m)
		val := pb.GetGauge().GetValue()

		switch {
		case strings.Contains(desc, "esxport_resourcepool_cpu_reservation_mhz"):
			found["cpu_reservation"] = val
		case strings.Contains(desc, "esxport_resourcepool_cpu_usage_mhz"):
			found["cpu_usage"] = val
		case strings.Contains(desc, "esxport_resourcepool_cpu_limit_mhz"):
			found["cpu_limit"] = val
		case strings.Contains(desc, "esxport_resourcepool_memory_reservation_bytes"):
			found["mem_reservation"] = val
		case strings.Contains(desc, "esxport_resourcepool_memory_usage_bytes"):
			found["mem_usage"] = val
		case strings.Contains(desc, "esxport_resourcepool_memory_limit_bytes"):
			found["mem_limit"] = val
		}
	}

	if found["cpu_usage"] != 2400 {
		t.Errorf("expected cpu_usage=2400, got %v", found["cpu_usage"])
	}
	if found["cpu_limit"] != 8000 {
		t.Errorf("expected cpu_limit=8000, got %v", found["cpu_limit"])
	}
	if found["cpu_reservation"] != 1000 {
		t.Errorf("expected cpu_reservation=1000, got %v", found["cpu_reservation"])
	}
	if found["mem_usage"] != 8589934592 {
		t.Errorf("expected mem_usage=8589934592, got %v", found["mem_usage"])
	}
	if found["mem_limit"] != 17179869184 {
		t.Errorf("expected mem_limit=17179869184, got %v", found["mem_limit"])
	}
	if found["mem_reservation"] != 4294967296 {
		t.Errorf("expected mem_reservation=4294967296, got %v", found["mem_reservation"])
	}
}

func TestResourcePoolUnlimitedValues(t *testing.T) {
	unlimited := int64(-1)
	pools := []mo.ResourcePool{
		{
			ManagedEntity: mo.ManagedEntity{ExtensibleManagedObject: mo.ExtensibleManagedObject{Self: types.ManagedObjectReference{Type: "ResourcePool", Value: "resgroup-456"}}, Name: "unlimited-pool"},
			Runtime: types.ResourcePoolRuntimeInfo{
				Cpu:    types.ResourcePoolResourceUsage{OverallUsage: 100},
				Memory: types.ResourcePoolResourceUsage{OverallUsage: 1024},
			},
			Config: types.ResourceConfigSpec{
				CpuAllocation:    types.ResourceAllocationInfo{Limit: &unlimited},
				MemoryAllocation: types.ResourceAllocationInfo{Limit: &unlimited},
			},
		},
	}

	client := &mockVSphereClient{
		resourcePools: pools,
	}

	target := config.Target{
		Host:    "vcenter.test",
		Collect: config.CollectConfig{ResourcePools: true},
	}

	ch := make(chan prometheus.Metric, 100)
	c := &Collector{}
	if err := c.collectResourcePools(context.Background(), ch, client, target); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	close(ch)

	for m := range ch {
		desc := m.Desc().String()
		pb := readMetric(t, m)
		val := pb.GetGauge().GetValue()

		if strings.Contains(desc, "limit") && val != -1 {
			t.Errorf("expected limit=-1 for unlimited, got %v in %s", val, desc)
		}
		if strings.Contains(desc, "reservation") && val != 0 {
			t.Errorf("expected reservation=0 when nil, got %v in %s", val, desc)
		}
	}
}
