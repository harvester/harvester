package definitions

import (
	"context"
	"time"

	"github.com/rancher/steve/pkg/debounce"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiregv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
)

// refreshHandler triggers refreshes for a Debounceable refresher after a CRD/APIService has been changed
// intended to refresh the schema definitions after a CRD has been added and is hopefully available in k8s.
type refreshHandler struct {
	// debounceRef is the debounceableRefresher containing the Refreshable (typically the schema definition handler)
	debounceRef *debounce.DebounceableRefresher
	// debounceDuration is the duration that the handler should ask the DebounceableRefresher to wait before refreshing
	debounceDuration time.Duration
}

// onChangeCRD refreshes the debounceRef after a CRD is added/changed
func (r *refreshHandler) onChangeCRD(key string, crd *apiextv1.CustomResourceDefinition) (*apiextv1.CustomResourceDefinition, error) {
	r.debounceRef.RefreshAfter(r.debounceDuration)
	return crd, nil
}

// onChangeAPIService refreshes the debounceRef after an APIService is added/changed
func (r *refreshHandler) onChangeAPIService(key string, api *apiregv1.APIService) (*apiregv1.APIService, error) {
	r.debounceRef.RefreshAfter(r.debounceDuration)
	return api, nil
}

// startBackgroundRefresh starts a force refresh that runs for every tick of duration. Can be stopped
// by cancelling the context
func (r *refreshHandler) startBackgroundRefresh(ctx context.Context, duration time.Duration) {
	go func() {
		ticker := time.NewTicker(duration)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.debounceRef.RefreshAfter(r.debounceDuration)
			}
		}
	}()
}
