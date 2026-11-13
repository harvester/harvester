package server

import (
	"context"
	"testing"

	"github.com/rancher/steve/pkg/sqlcache/informer"
	"github.com/rancher/steve/pkg/sqlcache/informer/factory"
	"github.com/rancher/steve/pkg/sqlcache/partition"
	"github.com/rancher/steve/pkg/sqlcache/sqltypes"
	"github.com/rancher/steve/pkg/stores/sqlproxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/harvester/harvester/pkg/util"
)

var vmGVK = schema.GroupVersionKind{Group: "kubevirt.io", Version: "v1", Kind: "VirtualMachine"}

func newTestVM(namespace, name, displayName, printableStatus string) *unstructured.Unstructured {
	vm := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kubevirt.io/v1",
		"kind":       "VirtualMachine",
		"metadata": map[string]any{
			"namespace":       namespace,
			"name":            name,
			"resourceVersion": "1",
		},
	}}
	if displayName != "" {
		vm.SetAnnotations(map[string]string{util.AnnotationVMDisplayName: displayName})
	}
	if printableStatus != "" {
		vm.Object["status"] = map[string]any{"printableStatus": printableStatus}
	}
	return vm
}

func listVMs(t *testing.T, cache *factory.Cache, opts sqltypes.ListOptions) ([]string, int, error) {
	t.Helper()

	list, total, _, err := cache.ListByOptions(t.Context(), &opts, []partition.Partition{{Passthrough: true}}, "")
	if err != nil {
		return nil, 0, err
	}
	ids := make([]string, 0, len(list.Items))
	for _, item := range list.Items {
		ids = append(ids, item.GetNamespace()+"/"+item.GetName())
	}
	return ids, total, nil
}

func TestRegisterSQLCacheIndexedFields(t *testing.T) {
	registerSQLCacheIndexedFields()
	registerSQLCacheIndexedFields()

	fields := sqlproxy.TypeSpecificIndexedFields[vmSQLCacheType]
	assert.Equal(t, sqlCacheIndexedFields[vmSQLCacheType], fields, "registering twice must not add duplicates")
}

// The VM fields are run through the real SQLite indexer of Steve. A field that the indexer cannot
// handle would keep the cache from syncing, and the VM list would never be served.
func TestVMIndexedFieldsWithSQLCache(t *testing.T) {
	registerSQLCacheIndexedFields()
	t.Chdir(t.TempDir())

	vms := []runtime.Object{
		newTestVM("default", "vm-a", "", "Running"),
		newTestVM("default", "vm-b", "Web Server", "Stopped"),
		newTestVM("default", "vm-c", "Database", "Running"),
		newTestVM("other", "vm-d", "web-proxy", "Provisioning"),
		newTestVM("other", "vm-e", "", ""),
	}

	gvr := schema.GroupVersionResource{Group: "kubevirt.io", Version: "v1", Resource: "virtualmachines"}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{gvr: "VirtualMachineList"},
		vms...,
	)

	cacheFactory, err := factory.NewCacheFactory(sqlCacheFactoryOptions)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	// Steve adds these two fields to every resource type, and sets "id" in its transform function.
	fields := append([][]string{{"id"}, {"metadata", "state", "name"}}, sqlproxy.TypeSpecificIndexedFields[vmSQLCacheType]...)
	setID := func(obj any) (any, error) {
		if u, ok := obj.(*unstructured.Unstructured); ok {
			u.Object["id"] = u.GetNamespace() + "/" + u.GetName()
		}
		return obj, nil
	}

	cache, err := cacheFactory.CacheFor(
		ctx,
		fields,
		nil, nil, setID,
		client.Resource(gvr),
		vmGVK,
		nil,
		true,
		true,
	)
	require.NoError(t, err)
	t.Cleanup(func() { cacheFactory.DoneWithCache(cache) })

	displayName := []string{"metadata", "annotations", util.AnnotationVMDisplayName}
	printableStatus := []string{"status", "printableStatus"}

	t.Run("all VMs are listed", func(t *testing.T) {
		ids, total, err := listVMs(t, cache, sqltypes.ListOptions{})
		require.NoError(t, err)
		assert.Equal(t, 5, total)
		assert.Equal(t, []string{"default/vm-a", "default/vm-b", "default/vm-c", "other/vm-d", "other/vm-e"}, ids)
	})

	t.Run("partial match on the display name", func(t *testing.T) {
		ids, _, err := listVMs(t, cache, sqltypes.ListOptions{
			Filters: []sqltypes.OrFilter{{Filters: []sqltypes.Filter{
				{Field: displayName, Matches: []string{"web"}, Op: sqltypes.Eq, Partial: true},
			}}},
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"default/vm-b", "other/vm-d"}, ids, "the partial match ignores the case")
	})

	t.Run("search matches the name or the display name", func(t *testing.T) {
		ids, _, err := listVMs(t, cache, sqltypes.ListOptions{
			Filters: []sqltypes.OrFilter{{Filters: []sqltypes.Filter{
				{Field: []string{"metadata", "name"}, Matches: []string{"vm-a"}, Op: sqltypes.Eq, Partial: true},
				{Field: displayName, Matches: []string{"Database"}, Op: sqltypes.Eq, Partial: true},
			}}},
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"default/vm-a", "default/vm-c"}, ids)
	})

	t.Run("exact match on the display name", func(t *testing.T) {
		ids, _, err := listVMs(t, cache, sqltypes.ListOptions{
			Filters: []sqltypes.OrFilter{{Filters: []sqltypes.Filter{
				{Field: displayName, Matches: []string{"web"}, Op: sqltypes.Eq},
			}}},
		})
		require.NoError(t, err)
		assert.Empty(t, ids)
	})

	t.Run("sort by printable status, then by name", func(t *testing.T) {
		ids, _, err := listVMs(t, cache, sqltypes.ListOptions{
			SortList: sqltypes.SortList{SortDirectives: []sqltypes.Sort{
				{Fields: printableStatus, Order: sqltypes.DESC},
				{Fields: []string{"metadata", "name"}, Order: sqltypes.ASC},
			}},
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"default/vm-b", "default/vm-a", "default/vm-c", "other/vm-d", "other/vm-e"}, ids)
	})

	t.Run("filter by namespace, sort and page", func(t *testing.T) {
		ids, total, err := listVMs(t, cache, sqltypes.ListOptions{
			Filters: []sqltypes.OrFilter{{Filters: []sqltypes.Filter{
				{Field: []string{"metadata", "namespace"}, Matches: []string{"default"}, Op: sqltypes.Eq},
			}}},
			SortList: sqltypes.SortList{SortDirectives: []sqltypes.Sort{
				{Fields: []string{"metadata", "name"}, Order: sqltypes.DESC},
			}},
			Pagination: sqltypes.Pagination{PageSize: 2, Page: 2},
		})
		require.NoError(t, err)
		assert.Equal(t, 3, total, "total is the number of filtered VMs, not of all VMs")
		assert.Equal(t, []string{"default/vm-a"}, ids)
	})

	t.Run("a field that is not indexed is rejected", func(t *testing.T) {
		_, _, err := listVMs(t, cache, sqltypes.ListOptions{
			SortList: sqltypes.SortList{SortDirectives: []sqltypes.Sort{
				{Fields: []string{"spec", "template", "spec", "domain", "cpu", "cores"}, Order: sqltypes.ASC},
			}},
		})
		assert.ErrorIs(t, err, informer.ErrInvalidColumn)
	})
}
