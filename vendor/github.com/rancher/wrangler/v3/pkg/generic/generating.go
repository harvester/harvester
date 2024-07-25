package generic

import (
	"github.com/rancher/wrangler/v3/pkg/apply"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type GeneratingHandlerOptions struct {
	AllowCrossNamespace bool
	AllowClusterScoped  bool
	NoOwnerReference    bool
	DynamicLookup       bool
	// UniqueApplyForResourceVersion will skip calling apply if the resource version didn't change from the previous execution
	UniqueApplyForResourceVersion bool
}

func ConfigureApplyForObject(apply apply.Apply, obj metav1.Object, opts *GeneratingHandlerOptions) apply.Apply {
	if opts == nil {
		opts = &GeneratingHandlerOptions{}
	}

	if opts.DynamicLookup {
		apply = apply.WithDynamicLookup()
	}

	if opts.NoOwnerReference {
		apply = apply.WithSetOwnerReference(true, false)
	}

	if opts.AllowCrossNamespace && !opts.AllowClusterScoped {
		apply = apply.
			WithDefaultNamespace(obj.GetNamespace()).
			WithListerNamespace(obj.GetNamespace())
	}

	if !opts.AllowClusterScoped {
		apply = apply.WithRestrictClusterScoped()
	}

	return apply
}
