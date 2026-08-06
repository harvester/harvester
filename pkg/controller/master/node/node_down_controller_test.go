package node

import (
	"testing"

	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	fakedynamic "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

// newUpgradePlan returns a minimal unstructured UpgradePlan with the given
// name and status.currentPhase. An empty phase mimics a freshly created plan
// whose status has not yet been reconciled.
func newUpgradePlan(name, phase string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   upgradePlanGVR.Group,
		Version: upgradePlanGVR.Version,
		Kind:    "UpgradePlan",
	})
	u.SetName(name)
	if phase != "" {
		_ = unstructured.SetNestedField(u.Object, phase, "status", "currentPhase")
	}
	return u
}

func newFakeDynamicClientWithPlans(plans ...*unstructured.Unstructured) *fakedynamic.FakeDynamicClient {
	gvrToListKind := map[schema.GroupVersionResource]string{
		upgradePlanGVR: "UpgradePlanList",
	}
	objs := make([]runtime.Object, 0, len(plans))
	for _, p := range plans {
		objs = append(objs, p)
	}
	return fakedynamic.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind, objs...)
}

func TestIsOnUpgradeV2(t *testing.T) {
	tests := []struct {
		name   string
		client func() dynamic.Interface
		want   bool
	}{
		{
			name:   "nil dynamic client returns false",
			client: func() dynamic.Interface { return nil },
			want:   false,
		},
		{
			name:   "no UpgradePlan exists returns false",
			client: func() dynamic.Interface { return newFakeDynamicClientWithPlans() },
			want:   false,
		},
		{
			name: "UpgradePlan in non-terminal phase returns true",
			client: func() dynamic.Interface {
				return newFakeDynamicClientWithPlans(newUpgradePlan("test-plan", "NodeUpgrading"))
			},
			want: true,
		},
		{
			name: "UpgradePlan with empty currentPhase (just created) returns true",
			client: func() dynamic.Interface {
				return newFakeDynamicClientWithPlans(newUpgradePlan("test-plan", ""))
			},
			want: true,
		},
		{
			name: "UpgradePlan with currentPhase=Succeeded returns false",
			client: func() dynamic.Interface {
				return newFakeDynamicClientWithPlans(newUpgradePlan("test-plan", upgradePlanPhaseSucceeded))
			},
			want: false,
		},
		{
			name: "UpgradePlan with currentPhase=Failed returns false",
			client: func() dynamic.Interface {
				return newFakeDynamicClientWithPlans(newUpgradePlan("test-plan", upgradePlanPhaseFailed))
			},
			want: false,
		},
		{
			name: "mix of terminal and non-terminal UpgradePlans returns true",
			client: func() dynamic.Interface {
				return newFakeDynamicClientWithPlans(
					newUpgradePlan("old-plan", upgradePlanPhaseSucceeded),
					newUpgradePlan("new-plan", "ImagePreloading"),
				)
			},
			want: true,
		},
		{
			name: "all UpgradePlans terminal returns false",
			client: func() dynamic.Interface {
				return newFakeDynamicClientWithPlans(
					newUpgradePlan("a", upgradePlanPhaseSucceeded),
					newUpgradePlan("b", upgradePlanPhaseFailed),
				)
			},
			want: false,
		},
		{
			name: "UpgradePlan CRD not registered (NoKindMatchError) returns false",
			client: func() dynamic.Interface {
				c := newFakeDynamicClientWithPlans()
				c.PrependReactor("list", "upgradeplans", func(_ k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, &meta.NoKindMatchError{
						GroupKind:        schema.GroupKind{Group: upgradePlanGVR.Group, Kind: "UpgradePlan"},
						SearchedVersions: []string{upgradePlanGVR.Version},
					}
				})
				return c
			},
			want: false,
		},
		{
			name: "UpgradePlan list returns NotFound returns false",
			client: func() dynamic.Interface {
				c := newFakeDynamicClientWithPlans()
				c.PrependReactor("list", "upgradeplans", func(_ k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, apierrors.NewNotFound(upgradePlanGVR.GroupResource(), "")
				})
				return c
			},
			want: false,
		},
		{
			name: "UpgradePlan list returns transient error returns false (fail-open)",
			client: func() dynamic.Interface {
				c := newFakeDynamicClientWithPlans()
				c.PrependReactor("list", "upgradeplans", func(_ k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, apierrors.NewServiceUnavailable("temporary glitch")
				})
				return c
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isOnUpgradeV2(tt.client())
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestIsOnUpgradeV2DoesNotPanicOnEmptyList ensures the fake's required
// list-kind registration is wired correctly and List succeeds even when no
// UpgradePlans exist.
func TestIsOnUpgradeV2DoesNotPanicOnEmptyList(t *testing.T) {
	c := newFakeDynamicClientWithPlans()
	assert.NotPanics(t, func() {
		_, err := c.Resource(upgradePlanGVR).List(t.Context(), metav1.ListOptions{})
		assert.NoError(t, err)
	})
}
