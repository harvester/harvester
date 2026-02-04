package proxy

import (
	"context"
	"time"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/accesscontrol"
	"k8s.io/apiserver/pkg/endpoints/request"
)

// WatchRefresh implements types.Store with awareness of changes to the requester's access.
type WatchRefresh struct {
	types.Store
	asl accesscontrol.AccessSetLookup
}

// NewWatchRefresh returns a new store with awareness of changes to the requester's access.
func NewWatchRefresh(s types.Store, asl accesscontrol.AccessSetLookup) *WatchRefresh {
	return &WatchRefresh{
		Store: s,
		asl:   asl,
	}
}

// Watch performs a watch request which halts if the user's access level changes.
func (w *WatchRefresh) Watch(apiOp *types.APIRequest, schema *types.APISchema, wr types.WatchRequest) (chan types.APIEvent, error) {
	user, ok := request.UserFrom(apiOp.Context())
	if !ok {
		return w.Store.Watch(apiOp, schema, wr)
	}

	as := w.asl.AccessFor(user)
	ctx, cancel := context.WithCancel(apiOp.Context())
	apiOp = apiOp.WithContext(ctx)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}

			newAs := w.asl.AccessFor(user)
			if as.ID != newAs.ID {
				// RBAC changed
				cancel()
				return
			}
		}
	}()

	return w.Store.Watch(apiOp, schema, wr)
}
