package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"

	"github.com/k8snetworkplumbingwg/whereabouts/pkg/allocate"
	whereaboutsv1alpha1 "github.com/k8snetworkplumbingwg/whereabouts/pkg/api/whereabouts.cni.cncf.io/v1alpha1"
	wbclient "github.com/k8snetworkplumbingwg/whereabouts/pkg/generated/clientset/versioned"
	"github.com/k8snetworkplumbingwg/whereabouts/pkg/iphelpers"
	"github.com/k8snetworkplumbingwg/whereabouts/pkg/logging"
	"github.com/k8snetworkplumbingwg/whereabouts/pkg/storage"
	whereaboutstypes "github.com/k8snetworkplumbingwg/whereabouts/pkg/types"
	"gomodules.xyz/jsonpatch/v2"
)

const UnnamedNetwork string = ""

// KubernetesIPAM manages ip blocks in an kubernetes CRD backend
type KubernetesIPAM struct {
	Client
	Config      whereaboutstypes.IPAMConfig
	Namespace   string
	ContainerID string
	IfName      string
}

func newKubernetesIPAM(containerID, ifName string, ipamConf whereaboutstypes.IPAMConfig, namespace string, kubernetesClient Client) *KubernetesIPAM {
	return &KubernetesIPAM{
		Config:      ipamConf,
		ContainerID: containerID,
		IfName:      ifName,
		Namespace:   namespace,
		Client:      kubernetesClient,
	}
}

// NewKubernetesIPAM returns a new KubernetesIPAM Client configured to a kubernetes CRD backend
func NewKubernetesIPAM(containerID, ifName string, ipamConf whereaboutstypes.IPAMConfig) (*KubernetesIPAM, error) {
	var namespace string
	if cfg, err := clientcmd.LoadFromFile(ipamConf.Kubernetes.KubeConfigPath); err != nil {
		return nil, err
	} else if ctx, ok := cfg.Contexts[cfg.CurrentContext]; ok && ctx != nil {
		namespace = wbNamespaceFromCtx(ctx)
	} else {
		return nil, fmt.Errorf("k8s config: namespace not present in context")
	}

	kubernetesClient, err := NewClientViaKubeconfig(ipamConf.Kubernetes.KubeConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed instantiating kubernetes client: %v", err)
	}
	k8sIPAM := newKubernetesIPAM(containerID, ifName, ipamConf, namespace, *kubernetesClient)
	return k8sIPAM, nil
}

// NewKubernetesIPAMWithNamespace returns a new KubernetesIPAM Client configured to a kubernetes CRD backend
func NewKubernetesIPAMWithNamespace(containerID, ifName string, ipamConf whereaboutstypes.IPAMConfig, namespace string) (*KubernetesIPAM, error) {
	k8sIPAM, err := NewKubernetesIPAM(containerID, ifName, ipamConf)
	if err != nil {
		return nil, err
	}
	k8sIPAM.Namespace = namespace
	return k8sIPAM, nil
}

type PoolIdentifier struct {
	IpRange     string
	NetworkName string
	NodeName    string
}

// GetIPPool returns a storage.IPPool for the given range
func (i *KubernetesIPAM) GetIPPool(ctx context.Context, poolIdentifier PoolIdentifier) (storage.IPPool, error) {
	name := IPPoolName(poolIdentifier)

	pool, err := i.getPool(ctx, name, poolIdentifier.IpRange)
	if err != nil {
		return nil, err
	}

	firstIP, _, err := pool.ParseCIDR()
	if err != nil {
		return nil, err
	}

	return &KubernetesIPPool{i.client, firstIP, pool}, nil
}

func IPPoolName(poolIdentifier PoolIdentifier) string {
	if poolIdentifier.NodeName != "" {
		// fast node range naming convention
		if poolIdentifier.NetworkName == UnnamedNetwork {
			return fmt.Sprintf("%v-%v", poolIdentifier.NodeName, normalizeRange(poolIdentifier.IpRange))
		} else {
			return fmt.Sprintf("%v-%v-%v", poolIdentifier.NetworkName, poolIdentifier.NodeName, normalizeRange(poolIdentifier.IpRange))
		}
	} else {
		// default naming convention
		if poolIdentifier.NetworkName == UnnamedNetwork {
			return normalizeRange(poolIdentifier.IpRange)
		} else {
			return fmt.Sprintf("%s-%s", poolIdentifier.NetworkName, normalizeRange(poolIdentifier.IpRange))
		}
	}
}

func normalizeRange(ipRange string) string {
	// v6 filter
	if ipRange[len(ipRange)-1] == ':' {
		ipRange = ipRange + "0"
	}
	normalized := strings.ReplaceAll(ipRange, ":", "-")

	// replace subnet cidr slash
	normalized = strings.ReplaceAll(normalized, "/", "-")
	return normalized
}

func (i *KubernetesIPAM) getPool(ctx context.Context, name string, iprange string) (*whereaboutsv1alpha1.IPPool, error) {
	ctxWithTimeout, cancel := context.WithTimeout(ctx, storage.RequestTimeout)
	defer cancel()

	pool, err := i.client.WhereaboutsV1alpha1().IPPools(i.Namespace).Get(ctxWithTimeout, name, metav1.GetOptions{})
	if err != nil && errors.IsNotFound(err) {
		// pool does not exist, create it
		newPool := &whereaboutsv1alpha1.IPPool{}
		newPool.ObjectMeta.Name = name
		newPool.Spec.Range = iprange
		newPool.Spec.Allocations = make(map[string]whereaboutsv1alpha1.IPAllocation)
		_, err = i.client.WhereaboutsV1alpha1().IPPools(i.Namespace).Create(ctxWithTimeout, newPool, metav1.CreateOptions{})
		if err != nil && errors.IsAlreadyExists(err) {
			// the pool was just created -- allow retry
			return nil, &temporaryError{err}
		} else if err != nil {
			return nil, fmt.Errorf("k8s create error: %s", err)
		}
		// if the pool was created for the first time, trigger another retry of the allocation loop
		// so all of the metadata / resourceVersions are populated as necessary by the `client.Get` call
		return nil, &temporaryError{fmt.Errorf("k8s pool initialized")}
	} else if err != nil {
		return nil, fmt.Errorf("k8s get error: %s", err)
	}
	return pool, nil
}

// Status tests connectivity to the kubernetes backend
func (i *KubernetesIPAM) Status(ctx context.Context) error {
	_, err := i.client.WhereaboutsV1alpha1().IPPools(i.Namespace).List(ctx, metav1.ListOptions{})
	return err
}

// Close partially implements the Store interface
func (i *KubernetesIPAM) Close() error {
	return nil
}

// KubernetesIPPool represents an IPPool resource and its parsed set of allocations
type KubernetesIPPool struct {
	client  wbclient.Interface
	firstIP net.IP
	pool    *whereaboutsv1alpha1.IPPool
}

// Allocations returns the initially retrieved set of allocations for this pool
func (p *KubernetesIPPool) Allocations() []whereaboutstypes.IPReservation {
	return toIPReservationList(p.pool.Spec.Allocations, p.firstIP)
}

// Update sets the pool allocated IP list to the given IP reservations
func (p *KubernetesIPPool) Update(ctx context.Context, reservations []whereaboutstypes.IPReservation) error {
	// marshal the current pool to serve as the base for the patch creation
	orig := p.pool.DeepCopy()
	origBytes, err := json.Marshal(orig)
	if err != nil {
		return err
	}

	// update the pool before marshalling once again
	allocations, err := toAllocationMap(reservations, p.firstIP)
	if err != nil {
		return err
	}
	p.pool.Spec.Allocations = allocations
	modBytes, err := json.Marshal(p.pool)
	if err != nil {
		return err
	}

	// create the patch
	patch, err := jsonpatch.CreatePatch(origBytes, modBytes)
	if err != nil {
		return err
	}

	// add additional tests to the patch
	ops := []jsonpatch.Operation{
		// ensure patch is applied to appropriate resource version only
		{Operation: "test", Path: "/metadata/resourceVersion", Value: orig.ObjectMeta.ResourceVersion},
	}
	for _, o := range patch {
		// safeguard add ops -- "add" will update existing paths, this "test" ensures the path is empty
		if o.Operation == "add" {
			var m map[string]interface{}
			ops = append(ops, jsonpatch.Operation{Operation: "test", Path: o.Path, Value: m})
		}
	}
	ops = append(ops, patch...)
	patchData, err := json.Marshal(ops)
	if err != nil {
		return err
	}

	// apply the patch
	_, err = p.client.WhereaboutsV1alpha1().IPPools(orig.GetNamespace()).Patch(ctx, orig.GetName(), types.JSONPatchType, patchData, metav1.PatchOptions{})
	if err != nil {
		if errors.IsInvalid(err) {
			// expect "invalid" errors if any of the jsonpatch "test" Operations fail
			return &temporaryError{err}
		}
		return err
	}

	return nil
}

func toIPReservationList(allocations map[string]whereaboutsv1alpha1.IPAllocation, firstip net.IP) []whereaboutstypes.IPReservation {
	reservelist := []whereaboutstypes.IPReservation{}
	for offset, a := range allocations {
		numOffset, err := strconv.ParseInt(offset, 10, 64)
		if err != nil {
			// allocations that are invalid int64s should be ignored
			// toAllocationMap should be the only writer of offsets, via `fmt.Sprintf("%d", ...)``
			logging.Errorf("Error decoding ip offset (backend: kubernetes): %v", err)
			continue
		}
		ip := iphelpers.IPAddOffset(firstip, uint64(numOffset))
		reservelist = append(reservelist, whereaboutstypes.IPReservation{IP: ip, ContainerID: a.ContainerID, PodRef: a.PodRef, IfName: a.IfName})
	}
	return reservelist
}

func toAllocationMap(reservelist []whereaboutstypes.IPReservation, firstip net.IP) (map[string]whereaboutsv1alpha1.IPAllocation, error) {
	allocations := make(map[string]whereaboutsv1alpha1.IPAllocation)
	for _, r := range reservelist {
		index, err := iphelpers.IPGetOffset(r.IP, firstip)
		if err != nil {
			return nil, err
		}
		allocations[fmt.Sprintf("%d", index)] = whereaboutsv1alpha1.IPAllocation{ContainerID: r.ContainerID, PodRef: r.PodRef, IfName: r.IfName}
	}
	return allocations, nil
}

// KubernetesOverlappingRangeStore represents a OverlappingRangeStore interface
type KubernetesOverlappingRangeStore struct {
	client    wbclient.Interface
	namespace string
}

// GetOverlappingRangeStore returns a clusterstore interface
func (i *KubernetesIPAM) GetOverlappingRangeStore() (storage.OverlappingRangeStore, error) {
	return &KubernetesOverlappingRangeStore{i.client, i.Namespace}, nil
}

// IsAllocatedInOverlappingRange checks for IP addresses to see if they're allocated cluster wide, for overlapping
// ranges. First return value is true if the IP is allocated, second return value is true if the IP is allocated to the
// current podRef
func (c *KubernetesOverlappingRangeStore) GetOverlappingRangeIPReservation(ctx context.Context, ip net.IP,
	podRef, networkName string) (*whereaboutsv1alpha1.OverlappingRangeIPReservation, error) {
	normalizedIP := NormalizeIP(ip, networkName)

	logging.Debugf("Get overlappingRangewide allocation; normalized IP: %q, IP: %q, networkName: %q",
		normalizedIP, ip, networkName)

	r, err := c.client.WhereaboutsV1alpha1().OverlappingRangeIPReservations(c.namespace).Get(ctx, normalizedIP, metav1.GetOptions{})
	if err != nil && errors.IsNotFound(err) {
		// cluster ip reservation does not exist, this appears to be good news.
		return nil, nil
	} else if err != nil {
		logging.Errorf("k8s get OverlappingRangeIPReservation error: %s", err)
		return nil, fmt.Errorf("k8s get OverlappingRangeIPReservation error: %s", err)
	}

	logging.Debugf("Normalized IP is reserved; normalized IP: %q, IP: %q, networkName: %q",
		normalizedIP, ip, networkName)
	return r, nil
}

// UpdateOverlappingRangeAllocation updates clusterwide allocation for overlapping ranges.
func (c *KubernetesOverlappingRangeStore) UpdateOverlappingRangeAllocation(ctx context.Context, mode int, ip net.IP,
	podRef, ifName, networkName string) error {
	normalizedIP := NormalizeIP(ip, networkName)

	clusteripres := &whereaboutsv1alpha1.OverlappingRangeIPReservation{
		ObjectMeta: metav1.ObjectMeta{Name: normalizedIP, Namespace: c.namespace},
	}

	var err error
	var verb string
	switch mode {
	case whereaboutstypes.Allocate:
		// Put together our cluster ip reservation
		verb = "allocate"

		clusteripres.Spec = whereaboutsv1alpha1.OverlappingRangeIPReservationSpec{
			PodRef: podRef,
			IfName: ifName,
		}

		_, err = c.client.WhereaboutsV1alpha1().OverlappingRangeIPReservations(c.namespace).Create(
			ctx, clusteripres, metav1.CreateOptions{})

	case whereaboutstypes.Deallocate:
		verb = "deallocate"
		err = c.client.WhereaboutsV1alpha1().OverlappingRangeIPReservations(c.namespace).Delete(ctx, clusteripres.GetName(), metav1.DeleteOptions{})
	}

	if err != nil {
		return err
	}

	logging.Debugf("K8s UpdateOverlappingRangeAllocation success on %v: %+v", verb, clusteripres)
	return nil
}

// NormalizeIP normalizes the IP. This is important for IPv6 which doesn't make for valid CR names. It also allows us
// to add the network-name when it's different from the unnamed network.
func NormalizeIP(ip net.IP, networkName string) string {
	ipStr := fmt.Sprint(ip)
	if ipStr[len(ipStr)-1] == ':' {
		ipStr += "0"
		logging.Debugf("modified: %s", ipStr)
	}
	normalizedIP := strings.ReplaceAll(ipStr, ":", "-")
	if networkName != UnnamedNetwork {
		normalizedIP = fmt.Sprintf("%s-%s", networkName, normalizedIP)
	}
	return normalizedIP
}

// getNodeName prefers an OS env var of NODENAME, or, uses a file named ./nodename in the whereabouts configuration path.
func getNodeName(ipam *KubernetesIPAM) (string, error) {

	envName := os.Getenv("NODENAME")
	if envName != "" {
		return strings.TrimSpace(envName), nil
	}

	nodeNamePath := fmt.Sprintf("%s/%s", ipam.Config.ConfigurationPath, "nodename")
	file, err := os.Open(nodeNamePath)
	if err != nil {
		file, err = os.Open("/etc/hostname")
		if err != nil {
			logging.Errorf("Could not determine nodename and could not open /etc/hostname: %v", err)
			return "", err
		}
	}
	defer file.Close()

	// Read the contents of the file
	data := make([]byte, 1024) // Adjust the buffer size as needed
	n, err := file.Read(data)
	if err != nil {
		logging.Errorf("Error reading file /etc/hostname: %v", err)
	}

	// Convert bytes to string
	hostname := string(data[:n])
	hostname = strings.TrimSpace(hostname)
	logging.Debugf("discovered current hostname as: %s", hostname)
	return hostname, nil
}

// newLeaderElector creates a new leaderelection.LeaderElector and associated
// channels by which to observe elections and depositions.
func newLeaderElector(ctx context.Context, clientset kubernetes.Interface, namespace string, ipamConf *KubernetesIPAM) (*leaderelection.LeaderElector, chan struct{}, chan struct{}) {
	//log.WithField("context", "leaderelection")
	// leaderOK will block gRPC startup until it's closed.
	leaderOK := make(chan struct{})
	// deposed is closed by the leader election callback when
	// we are deposed as leader so that we can clean up.
	deposed := make(chan struct{})

	leaseName := "whereabouts"
	if ipamConf.Config.NodeSliceSize != "" {
		// we lock per IP Pool so just use the pool name for the lease name
		hostname, err := getNodeName(ipamConf)
		if err != nil {
			logging.Errorf("Failed to create leader elector: %v", err)
			return nil, leaderOK, deposed
		}
		nodeSliceRange, err := GetNodeSlicePoolRange(ctx, ipamConf, hostname)
		if err != nil {
			logging.Errorf("Failed to create leader elector: %v", err)
			return nil, leaderOK, deposed
		}
		leaseName = IPPoolName(PoolIdentifier{IpRange: nodeSliceRange, NodeName: hostname, NetworkName: ipamConf.Config.NetworkName})
	}
	logging.Debugf("using lease with name: %v", leaseName)

	var rl = &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      leaseName,
			Namespace: namespace,
		},
		Client: clientset.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: fmt.Sprintf("%s/%s", ipamConf.Config.PodNamespace, ipamConf.Config.PodName),
		},
	}

	// Make the leader elector, ready to be used in the Workgroup.
	// !bang
	le, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
		Lock:            rl,
		LeaseDuration:   time.Duration(ipamConf.Config.LeaderLeaseDuration) * time.Millisecond,
		RenewDeadline:   time.Duration(ipamConf.Config.LeaderRenewDeadline) * time.Millisecond,
		RetryPeriod:     time.Duration(ipamConf.Config.LeaderRetryPeriod) * time.Millisecond,
		ReleaseOnCancel: true,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(_ context.Context) {
				logging.Debugf("OnStartedLeading() called")
				close(leaderOK)
			},
			OnStoppedLeading: func() {
				logging.Debugf("OnStoppedLeading() called")
				// The context being canceled will trigger a handler that will
				// deal with being deposed.
				close(deposed)
			},
		},
	})
	if err != nil {
		logging.Errorf("Failed to create leader elector: %v", err)
		return nil, leaderOK, deposed
	}
	return le, leaderOK, deposed
}

// IPManagement manages ip allocation and deallocation from a storage perspective
func IPManagement(ctx context.Context, mode int, ipamConf whereaboutstypes.IPAMConfig, client *KubernetesIPAM) ([]net.IPNet, error) {
	var newips []net.IPNet

	if ipamConf.PodName == "" {
		return newips, fmt.Errorf("IPAM client initialization error: no pod name")
	}

	// setup leader election
	le, leader, deposed := newLeaderElector(ctx, client.clientSet, client.Namespace, client)
	var wg sync.WaitGroup
	wg.Add(2)

	stopM := make(chan struct{})
	result := make(chan error, 2)

	var err error
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				err = fmt.Errorf("time limit exceeded while waiting to become leader")
				stopM <- struct{}{}
				return
			case <-leader:
				logging.Debugf("Elected as leader, do processing")
				newips, err = IPManagementKubernetesUpdate(ctx, mode, client, ipamConf)
				stopM <- struct{}{}
				return
			case <-deposed:
				logging.Debugf("Deposed as leader, shutting down")
				result <- nil
				return
			}
		}
	}()

	go func() {
		defer wg.Done()
		res := make(chan error)
		leCtx, leCancel := context.WithCancel(context.Background())

		go func() {
			logging.Debugf("Started leader election")
			le.Run(leCtx)
			logging.Debugf("Finished leader election")
			res <- nil
		}()

		// wait for stop which tells us when IP allocation occurred or context deadline exceeded
		<-stopM
		// leCancel fn(leCtx)
		leCancel()
		result <- (<-res)
	}()
	wg.Wait()
	close(stopM)
	logging.Debugf("IPManagement: %v, %v", newips, err)
	return newips, err
}

func GetNodeSlicePoolRange(ctx context.Context, ipam *KubernetesIPAM, nodeName string) (string, error) {
	logging.Debugf("ipam namespace is %v", ipam.Namespace)
	nodeSlice, err := ipam.client.WhereaboutsV1alpha1().NodeSlicePools(ipam.Namespace).Get(ctx, getNodeSliceName(ipam), metav1.GetOptions{})
	if err != nil {
		logging.Errorf("error getting node slice %s/%s %v", ipam.Namespace, getNodeSliceName(ipam), err)
		return "", err
	}
	for _, allocation := range nodeSlice.Status.Allocations {
		if allocation.NodeName == nodeName {
			logging.Debugf("found matching node slice allocation for hostname %v: %v", nodeName, allocation)
			return allocation.SliceRange, nil
		}
	}
	logging.Errorf("error finding node within node slice allocations")
	return "", fmt.Errorf("no allocated node slice for node")
}

func getNodeSliceName(ipam *KubernetesIPAM) string {
	if ipam.Config.NetworkName == UnnamedNetwork {
		return ipam.Config.Name
	}
	return ipam.Config.NetworkName
}

// IPManagementKubernetesUpdate manages k8s updates
func IPManagementKubernetesUpdate(ctx context.Context, mode int, ipam *KubernetesIPAM, ipamConf whereaboutstypes.IPAMConfig) ([]net.IPNet, error) {
	logging.Debugf("IPManagement -- mode: %d / containerID: %q / podRef: %q / ifName: %q ", mode, ipam.ContainerID, ipamConf.GetPodRef(), ipam.IfName)

	var newips []net.IPNet
	var newip net.IPNet
	// Skip invalid modes
	switch mode {
	case whereaboutstypes.Allocate, whereaboutstypes.Deallocate:
	default:
		return newips, fmt.Errorf("got an unknown mode passed to IPManagement: %v", mode)
	}

	var overlappingrangestore storage.OverlappingRangeStore
	var pool storage.IPPool
	var err error

	requestCtx, requestCancel := context.WithTimeout(ctx, storage.RequestTimeout)
	defer requestCancel()

	// Check our connectivity first
	if err := ipam.Status(requestCtx); err != nil {
		logging.Errorf("IPAM connectivity error: %v", err)
		return newips, err
	}

	// handle the ip add/del until successful
	var overlappingrangeallocations []whereaboutstypes.IPReservation
	var ipforoverlappingrangeupdate net.IP
	skipOverlappingRangeUpdate := false
	for _, ipRange := range ipamConf.IPRanges {
	RETRYLOOP:
		for j := 0; j < storage.DatastoreRetries; j++ {
			select {
			case <-ctx.Done():
				break RETRYLOOP
			default:
				// retry the IPAM loop if the context has not been cancelled
			}
			overlappingrangestore, err = ipam.GetOverlappingRangeStore()
			if err != nil {
				logging.Errorf("IPAM error getting OverlappingRangeStore: %v", err)
				return newips, err
			}
			poolIdentifier := PoolIdentifier{IpRange: ipRange.Range, NetworkName: ipamConf.NetworkName}
			if ipamConf.NodeSliceSize != "" {
				hostname, err := getNodeName(ipam)
				if err != nil {
					logging.Errorf("Failed to get node hostname: %v", err)
					return newips, err
				}
				poolIdentifier.NodeName = hostname
				nodeSliceRange, err := GetNodeSlicePoolRange(ctx, ipam, hostname)
				if err != nil {
					return newips, err
				}
				_, ipNet, err := net.ParseCIDR(nodeSliceRange)
				if err != nil {
					logging.Errorf("Error parsing node slice cidr to net.IPNet: %v", err)
					return newips, err
				}
				poolIdentifier.IpRange = nodeSliceRange
				rangeStart, err := iphelpers.FirstUsableIP(*ipNet)
				if err != nil {
					logging.Errorf("Error parsing node slice cidr to range start: %v", err)
					return newips, err
				}
				rangeEnd, err := iphelpers.LastUsableIP(*ipNet)
				if err != nil {
					logging.Errorf("Error parsing node slice cidr to range start: %v", err)
					return newips, err
				}
				ipRange = whereaboutstypes.RangeConfiguration{
					Range:      ipRange.Range,
					RangeStart: rangeStart,
					RangeEnd:   rangeEnd,
				}
			}
			logging.Debugf("using pool identifier: %v", poolIdentifier)
			pool, err = ipam.GetIPPool(requestCtx, poolIdentifier)
			if err != nil {
				logging.Errorf("IPAM error reading pool allocations (attempt: %d): %v", j, err)
				if e, ok := err.(storage.Temporary); ok && e.Temporary() {
					continue
				}
				return newips, err
			}

			reservelist := pool.Allocations()
			reservelist = append(reservelist, overlappingrangeallocations...)
			var updatedreservelist []whereaboutstypes.IPReservation
			switch mode {
			case whereaboutstypes.Allocate:
				newip, updatedreservelist, err = allocate.AssignIP(ipRange, reservelist, ipam.ContainerID, ipamConf.GetPodRef(), ipam.IfName)
				if err != nil {
					logging.Errorf("Error assigning IP: %v", err)
					return newips, err
				}
				// Now check if this is allocated overlappingrange wide
				// When it's allocated overlappingrange wide, we add it to a local reserved list
				// And we try again.
				if ipamConf.OverlappingRanges {
					overlappingRangeIPReservation, err := overlappingrangestore.GetOverlappingRangeIPReservation(requestCtx, newip.IP,
						ipamConf.GetPodRef(), ipamConf.NetworkName)
					if err != nil {
						logging.Errorf("Error getting cluster wide IP allocation: %v", err)
						return newips, err
					}

					if overlappingRangeIPReservation != nil {
						if overlappingRangeIPReservation.Spec.PodRef != ipamConf.GetPodRef() {
							logging.Debugf("Continuing loop, IP is already allocated (possibly from another range): %v", newip)
							// We create "dummy" records here for evaluation, but, we need to filter those out later.
							overlappingrangeallocations = append(overlappingrangeallocations, whereaboutstypes.IPReservation{IP: newip.IP, IsAllocated: true})
							continue
						}

						skipOverlappingRangeUpdate = true
					}

					ipforoverlappingrangeupdate = newip.IP
				}

			case whereaboutstypes.Deallocate:
				updatedreservelist, ipforoverlappingrangeupdate = allocate.DeallocateIP(reservelist, ipam.ContainerID, ipam.IfName)
				if ipforoverlappingrangeupdate == nil {
					// Do not fail if allocation was not found.
					logging.Debugf("Failed to find allocation for container ID: %s", ipam.ContainerID)
					return nil, nil
				}
			}

			// Clean out any dummy records from the reservelist...
			var usereservelist []whereaboutstypes.IPReservation
			for _, rl := range updatedreservelist {
				if !rl.IsAllocated {
					usereservelist = append(usereservelist, rl)
				}
			}

			// Manual race condition testing
			if ipamConf.SleepForRace > 0 {
				time.Sleep(time.Duration(ipamConf.SleepForRace) * time.Second)
			}

			err = pool.Update(requestCtx, usereservelist)
			if err != nil {
				logging.Errorf("IPAM error updating pool (attempt: %d): %v", j, err)
				if e, ok := err.(storage.Temporary); ok && e.Temporary() {
					continue
				}
				break RETRYLOOP
			}
			break RETRYLOOP
		}

		if ipamConf.OverlappingRanges {
			if !skipOverlappingRangeUpdate {
				err = overlappingrangestore.UpdateOverlappingRangeAllocation(requestCtx, mode, ipforoverlappingrangeupdate,
					ipamConf.GetPodRef(), ipam.IfName, ipamConf.NetworkName)
				if err != nil {
					logging.Errorf("Error performing UpdateOverlappingRangeAllocation: %v", err)
					return newips, err
				}
			}
		}

		newips = append(newips, newip)
	}
	return newips, err
}

func wbNamespaceFromCtx(ctx *clientcmdapi.Context) string {
	namespace := ctx.Namespace
	if namespace == "" {
		return metav1.NamespaceSystem
	}
	return namespace
}
