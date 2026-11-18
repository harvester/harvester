package storage

import (
	"context"
	"github.com/k8snetworkplumbingwg/whereabouts/pkg/api/whereabouts.cni.cncf.io/v1alpha1"
	"net"
	"time"

	"github.com/k8snetworkplumbingwg/whereabouts/pkg/types"
)

var (
	// RequestTimeout defines how long the context timesout in
	RequestTimeout = 10 * time.Second

	// DatastoreRetries defines how many retries are attempted when updating the Pool
	DatastoreRetries  = 100
	PodRefreshRetries = 3
)

// IPPool is the interface that represents an manageable pool of allocated IPs
type IPPool interface {
	Allocations() []types.IPReservation
	Update(ctx context.Context, reservations []types.IPReservation) error
}

// Store is the interface that wraps the basic IP Allocation methods on the underlying storage backend
type Store interface {
	GetIPPool(ctx context.Context, ipRange string) (IPPool, error)
	GetOverlappingRangeStore() (OverlappingRangeStore, error)
	Status(ctx context.Context) error
	Close() error
}

// OverlappingRangeStore is an interface for wrapping overlappingrange storage options
type OverlappingRangeStore interface {
	GetOverlappingRangeIPReservation(ctx context.Context, ip net.IP, podRef, networkName string) (*v1alpha1.OverlappingRangeIPReservation, error)
	UpdateOverlappingRangeAllocation(ctx context.Context, mode int, ip net.IP, podRef, ifName, networkName string) error
}

type Temporary interface {
	Temporary() bool
}
