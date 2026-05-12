package vsphere

import (
	"context"
	"fmt"
	"net/url"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/performance"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
)

// NetworkStat holds processed network performance data for a VM.
type NetworkStat struct {
	VMName  string
	RxBytes int64
	TxBytes int64
}

// Client wraps govmomi to provide a simplified interface for metric collection.
type Client struct {
	client  *govmomi.Client
	finder  *find.Finder
	manager *view.Manager
	onClose func() // optional callback invoked on Close, used for testing
}

// ConnectConfig holds connection parameters.
type ConnectConfig struct {
	Host      string
	Username  string
	Password  string
	IgnoreSSL bool
}

// Connect establishes a connection to the vSphere endpoint.
func Connect(ctx context.Context, cfg ConnectConfig) (*Client, error) {
	u, err := soap.ParseURL(cfg.Host)
	if err != nil {
		return nil, fmt.Errorf("parsing vSphere URL %q: %w", cfg.Host, err)
	}
	u.User = url.UserPassword(cfg.Username, cfg.Password)

	c, err := govmomi.NewClient(ctx, u, cfg.IgnoreSSL)
	if err != nil {
		return nil, fmt.Errorf("connecting to vSphere %q: %w", cfg.Host, err)
	}

	finder := find.NewFinder(c.Client, true)
	manager := view.NewManager(c.Client)

	return &Client{
		client:  c,
		finder:  finder,
		manager: manager,
	}, nil
}

// Close disconnects from vSphere.
func (c *Client) Close(ctx context.Context) error {
	if c.onClose != nil {
		c.onClose()
	}
	if c.client != nil {
		return c.client.Logout(ctx)
	}
	return nil
}

// HostSystems retrieves all host system properties.
func (c *Client) HostSystems(ctx context.Context) ([]mo.HostSystem, error) {
	v, err := c.manager.CreateContainerView(ctx, c.client.ServiceContent.RootFolder, []string{"HostSystem"}, true)
	if err != nil {
		return nil, fmt.Errorf("creating HostSystem container view: %w", err)
	}
	defer func() { _ = v.Destroy(ctx) }()

	var hosts []mo.HostSystem
	if err := v.Retrieve(ctx, []string{"HostSystem"}, []string{"name", "summary", "runtime"}, &hosts); err != nil {
		return nil, fmt.Errorf("retrieving host systems: %w", err)
	}
	return hosts, nil
}

// VirtualMachines retrieves all VM properties.
func (c *Client) VirtualMachines(ctx context.Context) ([]mo.VirtualMachine, error) {
	v, err := c.manager.CreateContainerView(ctx, c.client.ServiceContent.RootFolder, []string{"VirtualMachine"}, true)
	if err != nil {
		return nil, fmt.Errorf("creating VirtualMachine container view: %w", err)
	}
	defer func() { _ = v.Destroy(ctx) }()

	var vms []mo.VirtualMachine
	if err := v.Retrieve(ctx, []string{"VirtualMachine"}, []string{"name", "summary", "config", "snapshot", "runtime", "guest"}, &vms); err != nil {
		return nil, fmt.Errorf("retrieving virtual machines: %w", err)
	}
	return vms, nil
}

// Datastores retrieves all datastore properties.
func (c *Client) Datastores(ctx context.Context) ([]mo.Datastore, error) {
	v, err := c.manager.CreateContainerView(ctx, c.client.ServiceContent.RootFolder, []string{"Datastore"}, true)
	if err != nil {
		return nil, fmt.Errorf("creating Datastore container view: %w", err)
	}
	defer func() { _ = v.Destroy(ctx) }()

	var datastores []mo.Datastore
	if err := v.Retrieve(ctx, []string{"Datastore"}, []string{"name", "summary"}, &datastores); err != nil {
		return nil, fmt.Errorf("retrieving datastores: %w", err)
	}
	return datastores, nil
}

// ClusterComputeResources retrieves all cluster properties.
func (c *Client) ClusterComputeResources(ctx context.Context) ([]mo.ClusterComputeResource, error) {
	v, err := c.manager.CreateContainerView(ctx, c.client.ServiceContent.RootFolder, []string{"ClusterComputeResource"}, true)
	if err != nil {
		return nil, fmt.Errorf("creating ClusterComputeResource container view: %w", err)
	}
	defer func() { _ = v.Destroy(ctx) }()

	var clusters []mo.ClusterComputeResource
	if err := v.Retrieve(ctx, []string{"ClusterComputeResource"}, []string{"name", "summary", "host"}, &clusters); err != nil {
		return nil, fmt.Errorf("retrieving cluster compute resources: %w", err)
	}
	return clusters, nil
}

// ResourcePools retrieves all resource pool properties.
func (c *Client) ResourcePools(ctx context.Context) ([]mo.ResourcePool, error) {
	v, err := c.manager.CreateContainerView(ctx, c.client.ServiceContent.RootFolder, []string{"ResourcePool"}, true)
	if err != nil {
		return nil, fmt.Errorf("creating ResourcePool container view: %w", err)
	}
	defer func() { _ = v.Destroy(ctx) }()

	var pools []mo.ResourcePool
	if err := v.Retrieve(ctx, []string{"ResourcePool"}, []string{"name", "runtime", "config"}, &pools); err != nil {
		return nil, fmt.Errorf("retrieving resource pools: %w", err)
	}
	return pools, nil
}

// QueryNetworkPerformance retrieves network rx/tx bytes for VMs using the Performance Manager.
func (c *Client) QueryNetworkPerformance(ctx context.Context, vms []mo.VirtualMachine) ([]NetworkStat, error) {
	if len(vms) == 0 {
		return nil, nil
	}

	perfManager := performance.NewManager(c.client.Client)

	refs := make([]types.ManagedObjectReference, 0, len(vms))
	nameByRef := make(map[types.ManagedObjectReference]string, len(vms))
	for _, vm := range vms {
		refs = append(refs, vm.Self)
		nameByRef[vm.Self] = vm.Name
	}

	spec := types.PerfQuerySpec{
		MaxSample:  1,
		IntervalId: 20,
		MetricId:   []types.PerfMetricId{{Instance: "*"}},
	}

	sample, err := perfManager.SampleByName(ctx, spec,
		[]string{"net.bytesRx.average", "net.bytesTx.average"}, refs)
	if err != nil {
		return nil, fmt.Errorf("querying network performance: %w", err)
	}

	series, err := perfManager.ToMetricSeries(ctx, sample)
	if err != nil {
		return nil, fmt.Errorf("converting network performance series: %w", err)
	}

	statsByVM := make(map[string]*NetworkStat, len(vms))
	for _, em := range series {
		vmName := nameByRef[em.Entity]
		if vmName == "" {
			continue
		}
		stat, ok := statsByVM[vmName]
		if !ok {
			stat = &NetworkStat{VMName: vmName}
			statsByVM[vmName] = stat
		}
		for _, v := range em.Value {
			if len(v.Value) == 0 {
				continue
			}
			val := v.Value[len(v.Value)-1]
			switch v.Name {
			case "net.bytesRx.average":
				stat.RxBytes += val
			case "net.bytesTx.average":
				stat.TxBytes += val
			}
		}
	}

	result := make([]NetworkStat, 0, len(statsByVM))
	for _, stat := range statsByVM {
		result = append(result, *stat)
	}
	return result, nil
}
