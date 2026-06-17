package informer

import (
	"k8s.io/client-go/tools/cache"
)

// Copied from Kubernetes' source, only available in k8s 1.35

type listWatcherWithWatchListSemanticsWrapper struct {
	*cache.ListWatch

	// unsupportedWatchListSemantics indicates whether a client explicitly does NOT support
	// WatchList semantics.
	//
	// Over the years, unit tests in kube have been written in many different ways.
	// After enabling the WatchListClient feature by default, existing tests started failing.
	// To avoid breaking lots of existing client-go users after upgrade,
	// we introduced this field as an opt-in.
	//
	// When true, the reflector disables WatchList even if the feature gate is enabled.
	unsupportedWatchListSemantics bool
}

func (lw *listWatcherWithWatchListSemanticsWrapper) IsWatchListSemanticsUnSupported() bool {
	return lw.unsupportedWatchListSemantics
}

// toListWatcherWithWatchListSemantics returns a ListerWatcher
// that knows whether the provided client explicitly
// does NOT support the WatchList semantics. This allows Reflectors
// to adapt their behavior based on client capabilities.
func toListWatcherWithWatchListSemantics(lw *cache.ListWatch, client any) cache.ListerWatcher {
	return &listWatcherWithWatchListSemanticsWrapper{
		lw,
		doesClientNotSupportWatchListSemantics(client),
	}
}

type unSupportedWatchListSemantics interface {
	IsWatchListSemanticsUnSupported() bool
}

// doesClientNotSupportWatchListSemantics reports whether the given client
// does NOT support WatchList semantics.
//
// A client does NOT support WatchList only if
// it implements `IsWatchListSemanticsUnSupported` and that returns true.
func doesClientNotSupportWatchListSemantics(client any) bool {
	lw, ok := client.(unSupportedWatchListSemantics)
	if !ok {
		return false
	}
	return lw.IsWatchListSemanticsUnSupported()
}
