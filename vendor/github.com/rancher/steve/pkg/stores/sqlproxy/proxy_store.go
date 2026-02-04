// Package sqlproxy implements the proxy store, which is responsible for either interfacing directly with the Kubernetes API,
// or in the case of List, interfacing with an on-disk cache of items in the Kubernetes API.
package sqlproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/accesscontrol"
	"github.com/rancher/steve/pkg/sqlcache/informer"
	"github.com/rancher/steve/pkg/sqlcache/informer/factory"
	"github.com/rancher/steve/pkg/sqlcache/partition"
	"github.com/rancher/steve/pkg/sqlcache/sqltypes"
	"github.com/rancher/steve/pkg/stores/queryhelper"
	"github.com/rancher/wrangler/v3/pkg/data"
	"github.com/rancher/wrangler/v3/pkg/kv"
	"github.com/rancher/wrangler/v3/pkg/schemas/validation"
	"github.com/rancher/wrangler/v3/pkg/summary"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"github.com/rancher/steve/pkg/attributes"
	controllerschema "github.com/rancher/steve/pkg/controllers/schema"
	"github.com/rancher/steve/pkg/resources/common"
	"github.com/rancher/steve/pkg/resources/virtual"
	virtualCommon "github.com/rancher/steve/pkg/resources/virtual/common"
	metricsStore "github.com/rancher/steve/pkg/stores/metrics"
	"github.com/rancher/steve/pkg/stores/sqlpartition/listprocessor"
	"github.com/rancher/steve/pkg/stores/sqlproxy/tablelistconvert"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/jsonpath"
)

const (
	watchTimeoutEnv            = "CATTLE_WATCH_TIMEOUT_SECONDS"
	errNamespaceRequired       = "metadata.namespace or apiOp.namespace are required"
	errResourceVersionRequired = "metadata.resourceVersion is required for update"
)

var (
	paramScheme = runtime.NewScheme()
	paramCodec  = runtime.NewParameterCodec(paramScheme)
	// Please keep the gvkKey entries in alphabetical order, on a field-by-field basis
	TypeSpecificIndexedFields = map[string][][]string{
		gvkKey("", "v1", "Event"): {
			{"_type"},
			{"involvedObject", "kind"},
			{"involvedObject", "uid"},
			{"message"},
			{"reason"},
		},
		gvkKey("", "v1", "Namespace"): {
			{"spec", "displayName"},
		},
		gvkKey("", "v1", "Node"): {
			{"status", "nodeInfo", "kubeletVersion"},
			{"status", "nodeInfo", "operatingSystem"}},
		gvkKey("", "v1", "PersistentVolume"): {
			{"status", "reason"},
			{"spec", "persistentVolumeReclaimPolicy"},
		},
		gvkKey("", "v1", "PersistentVolumeClaim"): {
			{"spec", "volumeName"}},
		gvkKey("", "v1", "Pod"): {
			{"spec", "containers", "image"},
			{"spec", "nodeName"},
			{"status", "podIP"},
		},
		gvkKey("", "v1", "ReplicationController"): {
			{"spec", "template", "spec", "containers", "image"}},
		gvkKey("", "v1", "Secret"): {
			{"_type"},
			{"metadata", "annotations", "management.cattle.io/project-scoped-secret-copy"},
			{"spec", "clusterName"},
			{"spec", "displayName"},
		},
		gvkKey("", "v1", "Service"): {
			{"spec", "clusterIP"},
			{"spec", "type"},
		},
		gvkKey("apps", "v1", "DaemonSet"): {
			{"metadata", "annotations", "field.cattle.io/publicEndpoints"},
			{"spec", "template", "spec", "containers", "image"},
		},
		gvkKey("apps", "v1", "Deployment"): {
			{"metadata", "annotations", "field.cattle.io/publicEndpoints"},
			{"spec", "template", "spec", "containers", "image"},
		},
		gvkKey("apps", "v1", "ReplicaSet"): {
			{"spec", "template", "spec", "containers", "image"},
		},
		gvkKey("apps", "v1", "StatefulSet"): {
			{"metadata", "annotations", "field.cattle.io/publicEndpoints"},
			{"spec", "template", "spec", "containers", "image"},
		},
		gvkKey("autoscaling", "v2", "HorizontalPodAutoscaler"): {
			{"spec", "scaleTargetRef", "name"},
			{"spec", "minReplicas"},
			{"spec", "maxReplicas"},
			{"status", "currentReplicas"},
		},
		gvkKey("batch", "v1", "CronJob"): {
			{"metadata", "annotations", "field.cattle.io/publicEndpoints"},
			{"spec", "jobTemplate", "spec", "template", "spec", "containers", "image"},
			{"status", "lastScheduleTime"},
			{"status", "lastSuccessfulTime"},
		},
		gvkKey("batch", "v1", "Job"): {
			{"metadata", "annotations", "field.cattle.io/publicEndpoints"},
			{"spec", "template", "spec", "containers", "image"},
		},
		gvkKey("catalog.cattle.io", "v1", "App"): {
			{"spec", "chart", "metadata", "name"},
		},
		gvkKey("catalog.cattle.io", "v1", "ClusterRepo"): {
			{"metadata", "annotations", "clusterrepo.cattle.io/hidden"},
			{"spec", "gitBranch"},
			{"spec", "gitRepo"},
		},
		gvkKey("catalog.cattle.io", "v1", "Operation"): {
			{"status", "action"},
			{"status", "namespace"},
			{"status", "releaseName"},
		},
		gvkKey("cluster.x-k8s.io", "v1beta1", "Machine"): {
			{"spec", "clusterName"}},
		gvkKey("cluster.x-k8s.io", "v1beta1", "MachineDeployment"): {
			{"spec", "clusterName"}},
		gvkKey("management.cattle.io", "v3", "Cluster"): {
			{"spec", "internal"},
			{"spec", "displayName"},
			{"status", "allocatable", "cpu"},
			{"status", "allocatable", "cpuRaw"},
			{"status", "allocatable", "memory"},
			{"status", "allocatable", "memoryRaw"},
			{"status", "allocatable", "pods"},
			{"status", "requested", "cpu"},
			{"status", "requested", "cpuRaw"},
			{"status", "requested", "memory"},
			{"status", "requested", "memoryRaw"},
			{"status", "requested", "pods"},
			{"status", "connected"},
			{"status", "provider"},
		},
		gvkKey("management.cattle.io", "v3", "ClusterRoleTemplateBinding"): {
			{"clusterName"},
			{"userName"},
			{"userPrincipalName"},
		},
		gvkKey("management.cattle.io", "v3", "GlobalRoleBinding"): {
			{"userName"},
			{"userPrincipalName"},
		},
		gvkKey("management.cattle.io", "v3", "Node"): {
			{"status", "nodeName"}},
		gvkKey("management.cattle.io", "v3", "NodePool"): {
			{"spec", "clusterName"}},
		gvkKey("management.cattle.io", "v3", "NodeTemplate"): {
			{"spec", "clusterName"}},
		gvkKey("management.cattle.io", "v3", "Project"): {
			{"spec", "clusterName"},
			{"spec", "displayName"},
		},
		gvkKey("management.cattle.io", "v3", "ProjectRoleTemplateBinding"): {
			{"userName"},
			{"userPrincipalName"},
		},
		gvkKey("management.cattle.io", "v3", "RoleTemplate"): {
			{"context"},
		},
		gvkKey("networking.k8s.io", "v1", "Ingress"): {
			{"spec", "rules", "host"},
			{"spec", "ingressClassName"},
		},
		gvkKey("provisioning.cattle.io", "v1", "Cluster"): {
			{"metadata", "annotations", "provisioning.cattle.io/management-cluster-display-name"},
			{"status", "allocatable", "cpu"},
			{"status", "allocatable", "cpuRaw"},
			{"status", "allocatable", "memory"},
			{"status", "allocatable", "memoryRaw"},
			{"status", "allocatable", "pods"},
			{"status", "clusterName"},
			{"status", "provider"},
			{"status", "requested", "cpu"},
			{"status", "requested", "cpuRaw"},
			{"status", "requested", "memory"},
			{"status", "requested", "memoryRaw"},
			{"status", "requested", "pods"},
		},
		gvkKey("rke.cattle.io", "v1", "ETCDSnapshot"): {
			{"snapshotFile", "createdAt"},
			{"spec", "clusterName"},
		},
		gvkKey("storage.k8s.io", "v1", "StorageClass"): {
			{"provisioner"},
			{"metadata", "annotations", "storageclass.kubernetes.io/is-default-class"},
		},
	}
	commonIndexFields = [][]string{
		{`id`},
		{`metadata`, `state`, `name`},
	}
	namespaceGVK             = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}
	mcioProjectGvk           = schema.GroupVersionKind{Group: "management.cattle.io", Version: "v3", Kind: "Project"}
	pcioClusterGvk           = schema.GroupVersionKind{Group: "provisioning.cattle.io", Version: "v1", Kind: "Cluster"}
	namespaceProjectLabelDep = sqltypes.ExternalLabelDependency{
		SourceGVK:            gvkKey("", "v1", "Namespace"),
		SourceLabelName:      "field.cattle.io/projectId",
		TargetGVK:            gvkKey("management.cattle.io", "v3", "Project"),
		TargetKeyFieldName:   "metadata.name",
		TargetFinalFieldName: "spec.displayName",
	}
	namespaceUpdates = sqltypes.ExternalGVKUpdates{
		AffectedGVK:               namespaceGVK,
		ExternalDependencies:      nil,
		ExternalLabelDependencies: []sqltypes.ExternalLabelDependency{namespaceProjectLabelDep},
	}

	secretGVK                    = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"}
	secretProjectLabelDisplayDep = sqltypes.ExternalLabelDependency{
		SourceGVK:            gvkKey("", "v1", "Secret"),
		SourceLabelName:      "management.cattle.io/project-scoped-secret",
		TargetGVK:            gvkKey("management.cattle.io", "v3", "Project"),
		TargetKeyFieldName:   "metadata.name",
		TargetFinalFieldName: "spec.displayName",
	}
	secretProjectLabelClusterDep = sqltypes.ExternalLabelDependency{
		SourceGVK:            gvkKey("", "v1", "Secret"),
		SourceLabelName:      "management.cattle.io/project-scoped-secret",
		TargetGVK:            gvkKey("management.cattle.io", "v3", "Project"),
		TargetKeyFieldName:   "metadata.name",
		TargetFinalFieldName: "spec.clusterName",
	}
	secretUpdates = sqltypes.ExternalGVKUpdates{
		AffectedGVK:               secretGVK,
		ExternalDependencies:      nil,
		ExternalLabelDependencies: []sqltypes.ExternalLabelDependency{secretProjectLabelDisplayDep, secretProjectLabelClusterDep},
	}

	// Now sort provisioned.cattle.io.clusters based on their associated mgmt.cattle.io spec values
	// We might need to pull in the `memoryRaw` fields as well
	// Remember to index these fields in the database.
	provisionedClusterDependencies = func() []sqltypes.ExternalDependency {
		fields := []string{
			"status.allocatable.cpu",
			"status.allocatable.cpuRaw",
			"status.allocatable.memory",
			"status.allocatable.memoryRaw",
			"status.allocatable.pods",
			"status.requested.cpu",
			"status.requested.cpuRaw",
			"status.requested.memory",
			"status.requested.memoryRaw",
			"status.requested.pods",
		}
		x := make([]sqltypes.ExternalDependency, len(fields))
		for i, field := range fields {
			x[i] = sqltypes.ExternalDependency{
				SourceGVK:            gvkKey("provisioning.cattle.io", "v1", "Cluster"),
				SourceFieldName:      "status.clusterName",
				TargetGVK:            gvkKey("management.cattle.io", "v3", "Cluster"),
				TargetKeyFieldName:   "id",
				TargetFinalFieldName: field,
			}
		}
		return x
	}()
	pcioClusterUpdates = sqltypes.ExternalGVKUpdates{
		AffectedGVK:               pcioClusterGvk,
		ExternalDependencies:      provisionedClusterDependencies,
		ExternalLabelDependencies: nil,
	}

	externalGVKDependencies = sqltypes.ExternalGVKDependency{
		mcioProjectGvk: &namespaceUpdates,
		pcioClusterGvk: &pcioClusterUpdates,
		secretGVK:      &secretUpdates,
	}

	selfGVKDependencies = sqltypes.ExternalGVKDependency{
		// When a namespace is updated, we need to pull in changes from mcio into the namespaces table
		namespaceGVK:   &namespaceUpdates,
		pcioClusterGvk: &pcioClusterUpdates,
		secretGVK:      &secretUpdates,
	}
)

func init() {
	metav1.AddToGroupVersion(paramScheme, metav1.SchemeGroupVersion)
}

// ClientGetter is a dynamic kubernetes client factory.
type ClientGetter interface {
	IsImpersonating() bool
	K8sInterface(ctx *types.APIRequest) (kubernetes.Interface, error)
	AdminK8sInterface() (kubernetes.Interface, error)
	Client(ctx *types.APIRequest, schema *types.APISchema, namespace string, warningHandler rest.WarningHandler) (dynamic.ResourceInterface, error)
	DynamicClient(ctx *types.APIRequest, warningHandler rest.WarningHandler) (dynamic.Interface, error)
	AdminClient(ctx *types.APIRequest, schema *types.APISchema, namespace string, warningHandler rest.WarningHandler) (dynamic.ResourceInterface, error)
	TableClient(ctx *types.APIRequest, schema *types.APISchema, namespace string, warningHandler rest.WarningHandler) (dynamic.ResourceInterface, error)
	TableAdminClient(ctx *types.APIRequest, schema *types.APISchema, namespace string, warningHandler rest.WarningHandler) (dynamic.ResourceInterface, error)
	TableClientForWatch(ctx *types.APIRequest, schema *types.APISchema, namespace string, warningHandler rest.WarningHandler) (dynamic.ResourceInterface, error)
	TableAdminClientForWatch(ctx *types.APIRequest, schema *types.APISchema, namespace string, warningHandler rest.WarningHandler) (dynamic.ResourceInterface, error)
}

type SchemaColumnSetter interface {
	SetColumns(ctx context.Context, schema *types.APISchema) error
}

type SchemaCollection interface {
	Schema(id string) *types.APISchema
	ByGVK(gvk schema.GroupVersionKind) string
}

type Cache interface {
	// ListByOptions returns objects according to the specified list options and partitions.
	// Specifically:
	//   - an unstructured list of resources belonging to any of the specified partitions
	//   - the total number of resources (returned list might be a subset depending on pagination options in lo)
	//   - a summary object, containing the possible values for each field specified in a summary= subquery
	//   - a continue token, if there are more pages after the returned one
	//   - an error instead of all of the above if anything went wrong
	ListByOptions(ctx context.Context, lo *sqltypes.ListOptions, partitions []partition.Partition, namespace string) (*unstructured.UnstructuredList, int, *types.APISummary, string, error)
}

// WarningBuffer holds warnings that may be returned from the kubernetes api
type WarningBuffer []types.Warning

// HandleWarningHeader takes the components of a kubernetes warning header and stores them
func (w *WarningBuffer) HandleWarningHeader(code int, agent string, text string) {
	*w = append(*w, types.Warning{
		Code:  code,
		Agent: agent,
		Text:  text,
	})
}

// RelationshipNotifier is an interface for handling wrangler summary.Relationship events.
type RelationshipNotifier interface {
	OnInboundRelationshipChange(ctx context.Context, schema *types.APISchema, namespace string) <-chan *summary.Relationship
}

type TransformBuilder interface {
	GetTransformFunc(gvk schema.GroupVersionKind, colDefs []common.ColumnDefinition, isCRD bool, jsonPaths map[string]*jsonpath.JSONPath) cache.TransformFunc
}

type Store struct {
	ctx              context.Context
	clientGetter     ClientGetter
	notifier         RelationshipNotifier
	cacheFactory     CacheFactory
	cfInitializer    CacheFactoryInitializer
	namespaceCache   *factory.Cache
	lock             sync.Mutex
	columnSetter     SchemaColumnSetter
	transformBuilder TransformBuilder
	schemas          SchemaCollection

	watchers *Watchers
}

type CacheFactoryInitializer func() (CacheFactory, error)

type CacheFactory interface {
	CacheFor(ctx context.Context, fields [][]string, externalUpdateInfo *sqltypes.ExternalGVKUpdates, selfUpdateInfo *sqltypes.ExternalGVKUpdates, transform cache.TransformFunc, client dynamic.ResourceInterface, gvk schema.GroupVersionKind, typeGuidance map[string]string, namespaced bool, watchable bool) (*factory.Cache, error)
	DoneWithCache(*factory.Cache)
	Stop(gvk schema.GroupVersionKind) error
}

// NewProxyStore returns a Store implemented directly on top of kubernetes.
func NewProxyStore(ctx context.Context, c SchemaColumnSetter, clientGetter ClientGetter, notifier RelationshipNotifier, scache virtualCommon.SummaryCache, schemas SchemaCollection, factory CacheFactory, needToInitNamespaceCache bool) (*Store, error) {
	store := &Store{
		ctx:              ctx,
		clientGetter:     clientGetter,
		notifier:         notifier,
		columnSetter:     c,
		transformBuilder: virtual.NewTransformBuilder(scache),
		schemas:          schemas,
		watchers:         newWatchers(),
	}

	if factory == nil {
		var err error
		factory, err = defaultInitializeCacheFactory()
		if err != nil {
			return nil, err
		}
	}

	store.cacheFactory = factory
	if needToInitNamespaceCache {
		if err := store.initializeNamespaceCache(); err != nil {
			logrus.Infof("failed to warm up namespace informer for proxy store in steve, will try again on next ns request")
		}
	}
	return store, nil
}

// Reset locks the store, resets the underlying cache factory, and warm the namespace cache.
func (s *Store) Reset(gvk schema.GroupVersionKind) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.namespaceCache != nil && gvk == namespaceGVK {
		s.cacheFactory.DoneWithCache(s.namespaceCache)
	}
	if err := s.cacheFactory.Stop(gvk); err != nil {
		return fmt.Errorf("reset: %w", err)
	}

	if gvk == namespaceGVK {
		if err := s.initializeNamespaceCache(); err != nil {
			return err
		}
	}
	return nil
}

func defaultInitializeCacheFactory() (CacheFactory, error) {
	informerFactory, err := factory.NewCacheFactory(factory.CacheFactoryOptions{})
	if err != nil {
		return nil, err
	}
	return informerFactory, nil
}

// initializeNamespaceCache warms up the namespace cache as it is needed to process queries using options related to
// namespaces and projects.
func (s *Store) initializeNamespaceCache() error {
	buffer := WarningBuffer{}
	var nsSchema *types.APISchema

	if id := s.schemas.ByGVK(namespaceGVK); id != "" {
		nsSchema = s.schemas.Schema(id)
	}

	if nsSchema == nil {
		return fmt.Errorf("namespace schema not found")
	}

	// make sure any relevant columns are set to the ns schema
	if err := s.columnSetter.SetColumns(s.ctx, nsSchema); err != nil {
		return fmt.Errorf("failed to set columns for proxy stores namespace informer: %w", err)
	}

	// build table client
	client, err := s.clientGetter.TableAdminClient(nil, nsSchema, "", &buffer)
	if err != nil {
		return err
	}

	gvk := attributes.GVK(nsSchema)
	fields, cols, typeGuidance := getFieldAndColInfo(nsSchema, gvk)
	// get any type-specific fields that steve is interested in
	fields = append(fields, getFieldForGVK(gvk)...)

	// get the type-specific transform func
	transformFunc := s.transformBuilder.GetTransformFunc(gvk, cols, attributes.IsCRD(nsSchema), attributes.CRDJSONPathParsers(nsSchema))

	// get the ns informer
	tableClient := &tablelistconvert.Client{ResourceInterface: client}
	nsInformer, err := s.cacheFactory.CacheFor(s.ctx,
		fields,
		externalGVKDependencies[gvk],
		selfGVKDependencies[gvk],
		transformFunc,
		tableClient,
		gvk,
		typeGuidance,
		false,
		true)
	if err != nil {
		return err
	}

	s.namespaceCache = nsInformer
	return nil
}

func getFieldForGVK(gvk schema.GroupVersionKind) [][]string {
	fields := [][]string{}
	fields = append(fields, commonIndexFields...)
	typeFields := TypeSpecificIndexedFields[gvkKey(gvk.Group, gvk.Version, gvk.Kind)]
	if typeFields != nil {
		fields = append(fields, typeFields...)
	}
	return fields
}

func gvkKey(group, version, kind string) string {
	return group + "_" + version + "_" + kind
}

// getFieldAndColInfo converts object field names from types.APISchema's format into steve's
// cache.sql.informer's slice format (e.g. "metadata.resourceVersion" is ["metadata", "resourceVersion"])
// It also returns type info for each field
func getFieldAndColInfo(schema *types.APISchema, gvk schema.GroupVersionKind) ([][]string, []common.ColumnDefinition, map[string]string) {
	var fields [][]string
	colDefs := common.GetColumnDefinitions(schema)
	if colDefs != nil {
		for _, colDef := range colDefs {
			field := strings.TrimPrefix(colDef.Field, "$")
			field = strings.TrimPrefix(field, ".")
			fields = append(fields, queryhelper.SafeSplit(field))
		}
	}
	typeGuidance := getTypeGuidance(colDefs, gvk)

	return fields, colDefs, typeGuidance
}

// ByID looks up a single object by its ID.
func (s *Store) ByID(apiOp *types.APIRequest, schema *types.APISchema, id string) (*unstructured.Unstructured, []types.Warning, error) {
	return s.byID(apiOp, schema, apiOp.Namespace, id)
}

func decodeParams(apiOp *types.APIRequest, target runtime.Object) error {
	return paramCodec.DecodeParameters(apiOp.Request.URL.Query(), metav1.SchemeGroupVersion, target)
}

func (s *Store) byID(apiOp *types.APIRequest, schema *types.APISchema, namespace, id string) (*unstructured.Unstructured, []types.Warning, error) {
	buffer := WarningBuffer{}
	k8sClient, err := metricsStore.Wrap(s.clientGetter.TableClient(apiOp, schema, namespace, &buffer))
	if err != nil {
		return nil, nil, err
	}

	opts := metav1.GetOptions{}
	if err := decodeParams(apiOp, &opts); err != nil {
		return nil, nil, err
	}

	obj, err := k8sClient.Get(apiOp, id, opts)
	rowToObject(obj)
	return obj, buffer, err
}

func moveFromUnderscore(obj map[string]interface{}) map[string]interface{} {
	if obj == nil {
		return nil
	}
	for k := range types.ReservedFields {
		v, ok := obj["_"+k]
		delete(obj, "_"+k)
		delete(obj, k)
		if ok {
			obj[k] = v
		}
	}
	return obj
}

func rowToObject(obj *unstructured.Unstructured) {
	if obj == nil {
		return
	}
	if obj.Object["kind"] != "Table" ||
		(obj.Object["apiVersion"] != "meta.k8s.io/v1" &&
			obj.Object["apiVersion"] != "meta.k8s.io/v1beta1") {
		return
	}

	items := tableToObjects(obj.Object)
	if len(items) == 1 {
		obj.Object = items[0].Object
	}
}

func tableToObjects(obj map[string]interface{}) []unstructured.Unstructured {
	var result []unstructured.Unstructured

	rows, _ := obj["rows"].([]interface{})
	for _, row := range rows {
		m, ok := row.(map[string]interface{})
		if !ok {
			continue
		}
		cells := m["cells"]
		object, ok := m["object"].(map[string]interface{})
		if !ok {
			continue
		}

		data.PutValue(object, cells, "metadata", "fields")
		result = append(result, unstructured.Unstructured{
			Object: object,
		})
	}

	return result
}

func returnErr(err error, c chan watch.Event) {
	c <- watch.Event{
		Type: watch.Error,
		Object: &metav1.Status{
			Message: err.Error(),
		},
	}
}

// WatchNames returns a channel of events filtered by an allowed set of names.
// In plain kubernetes, if a user has permission to 'list' or 'watch' a defined set of resource names,
// performing the list or watch will result in a Forbidden error, because the user does not have permission
// to list *all* resources.
// With this filter, the request can be performed successfully, and only the allowed resources will
// be returned in watch.
func (s *Store) WatchNames(apiOp *types.APIRequest, schema *types.APISchema, w types.WatchRequest, names sets.Set[string]) (chan watch.Event, error) {
	buffer := &WarningBuffer{}
	adminClient, err := s.clientGetter.TableAdminClientForWatch(apiOp, schema, "", buffer)
	if err != nil {
		return nil, err
	}
	c, err := s.watch(apiOp, schema, w, adminClient)
	if err != nil {
		return nil, err
	}

	result := make(chan watch.Event)
	go func() {
		defer close(result)
		for item := range c {
			if item.Type == watch.Error {
				if status, ok := item.Object.(*metav1.Status); ok {
					logrus.Debugf("WatchNames received error: %s", status.Message)
				} else {
					logrus.Debugf("WatchNames received error: %v", item)
				}
				result <- item
				continue
			}

			m, err := meta.Accessor(item.Object)
			if err != nil {
				logrus.Debugf("WatchNames cannot process unexpected object: %s", err)
				continue
			}

			if names.Has(m.GetName()) {
				result <- item
			}
		}
	}()

	return result, nil
}

// Watch returns a channel of events for a list or resource.
func (s *Store) Watch(apiOp *types.APIRequest, schema *types.APISchema, w types.WatchRequest) (chan watch.Event, error) {
	buffer := &WarningBuffer{}
	client, err := s.clientGetter.TableAdminClientForWatch(apiOp, schema, "", buffer)
	if err != nil {
		return nil, err
	}
	return s.watch(apiOp, schema, w, client)
}

type Watchers struct {
	watchers map[string]struct{}
}

func newWatchers() *Watchers {
	return &Watchers{
		watchers: make(map[string]struct{}),
	}
}

func (s *Store) watch(apiOp *types.APIRequest, schema *types.APISchema, w types.WatchRequest, client dynamic.ResourceInterface) (chan watch.Event, error) {
	ctx := apiOp.Context()
	inf, doneFn, err := s.cacheForWithDeps(ctx, apiOp, schema)
	if err != nil {
		return nil, err
	}

	// Cancel watch if the informer is shutdown
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-inf.Context().Done():
			cancel()
		case <-ctx.Done():
		}
	}()

	var selector labels.Selector
	if w.Selector != "" {
		selector, err = labels.Parse(w.Selector)
		if err != nil {
			return nil, fmt.Errorf("invalid selector: %w", err)
		}
	}

	result := make(chan watch.Event)
	go func() {
		defer cancel()
		defer doneFn()
		defer close(result)

		idNamespace, _ := kv.RSplit(w.ID, "/")
		if idNamespace == "" {
			idNamespace = apiOp.Namespace
		}

		opts := informer.WatchOptions{
			ResourceVersion: w.Revision,
			Filter: informer.WatchFilter{
				ID:        w.ID,
				Namespace: idNamespace,
				Selector:  selector,
			},
		}
		err := inf.ByOptionsLister.Watch(ctx, opts, result)
		if err != nil {
			returnErr(err, result)
			return
		}
		logrus.Debugf("closing watcher for %s", schema.ID)
	}()
	return result, nil
}

// Create creates a single object in the store.
func (s *Store) Create(apiOp *types.APIRequest, schema *types.APISchema, params types.APIObject) (*unstructured.Unstructured, []types.Warning, error) {
	var (
		resp *unstructured.Unstructured
	)

	input := params.Data()

	if input == nil {
		input = data.Object{}
	}

	name := types.Name(input)
	namespace := types.Namespace(input)
	generateName := input.String("metadata", "generateName")

	if name == "" && generateName == "" {
		input.SetNested(schema.ID[0:1]+"-", "metadata", "generateName")
	}

	if attributes.Namespaced(schema) && namespace == "" {
		if apiOp.Namespace == "" {
			return nil, nil, apierror.NewAPIError(validation.InvalidBodyContent, errNamespaceRequired)
		}

		namespace = apiOp.Namespace
		input.SetNested(namespace, "metadata", "namespace")
	}

	gvk := attributes.GVK(schema)
	input["apiVersion"], input["kind"] = gvk.ToAPIVersionAndKind()

	buffer := WarningBuffer{}
	k8sClient, err := metricsStore.Wrap(s.clientGetter.Client(apiOp, schema, namespace, &buffer))
	if err != nil {
		return nil, nil, err
	}

	opts := metav1.CreateOptions{}
	if err := decodeParams(apiOp, &opts); err != nil {
		return nil, nil, err
	}

	resp, err = k8sClient.Create(apiOp, &unstructured.Unstructured{Object: input}, opts)
	rowToObject(resp)
	return resp, buffer, err
}

// Update updates a single object in the store.
func (s *Store) Update(apiOp *types.APIRequest, schema *types.APISchema, params types.APIObject, id string) (*unstructured.Unstructured, []types.Warning, error) {
	var (
		err   error
		input = params.Data()
	)

	ns := types.Namespace(input)
	buffer := WarningBuffer{}
	k8sClient, err := metricsStore.Wrap(s.clientGetter.Client(apiOp, schema, ns, &buffer))
	if err != nil {
		return nil, nil, err
	}

	if apiOp.Method == http.MethodPatch {
		bytes, err := ioutil.ReadAll(io.LimitReader(apiOp.Request.Body, 2<<20))
		if err != nil {
			return nil, nil, err
		}

		pType := apitypes.StrategicMergePatchType
		if apiOp.Request.Header.Get("content-type") == string(apitypes.JSONPatchType) {
			pType = apitypes.JSONPatchType
		}

		opts := metav1.PatchOptions{}
		if err := decodeParams(apiOp, &opts); err != nil {
			return nil, nil, err
		}

		if pType == apitypes.StrategicMergePatchType {
			data := map[string]interface{}{}
			if err := json.Unmarshal(bytes, &data); err != nil {
				return nil, nil, err
			}
			data = moveFromUnderscore(data)
			bytes, err = json.Marshal(data)
			if err != nil {
				return nil, nil, err
			}
		}

		resp, err := k8sClient.Patch(apiOp, id, pType, bytes, opts)
		if err != nil {
			return nil, nil, err
		}

		return resp, buffer, nil
	}

	resourceVersion := input.String("metadata", "resourceVersion")
	if resourceVersion == "" {
		return nil, nil, errors.New(errResourceVersionRequired)
	}

	gvk := attributes.GVK(schema)
	input["apiVersion"], input["kind"] = gvk.ToAPIVersionAndKind()

	opts := metav1.UpdateOptions{}
	if err := decodeParams(apiOp, &opts); err != nil {
		return nil, nil, err
	}

	resp, err := k8sClient.Update(apiOp, &unstructured.Unstructured{Object: moveFromUnderscore(input)}, metav1.UpdateOptions{})
	if err != nil {
		return nil, nil, err
	}

	rowToObject(resp)
	return resp, buffer, nil
}

// Delete deletes an object from a store.
func (s *Store) Delete(apiOp *types.APIRequest, schema *types.APISchema, id string) (*unstructured.Unstructured, []types.Warning, error) {
	opts := metav1.DeleteOptions{}
	if err := decodeParams(apiOp, &opts); err != nil {
		return nil, nil, nil
	}

	buffer := WarningBuffer{}
	k8sClient, err := metricsStore.Wrap(s.clientGetter.Client(apiOp, schema, apiOp.Namespace, &buffer))
	if err != nil {
		return nil, nil, err
	}

	if err := k8sClient.Delete(apiOp, id, opts); err != nil {
		return nil, nil, err
	}

	obj, _, err := s.byID(apiOp, schema, apiOp.Namespace, id)
	if err != nil {
		// ignore lookup error
		return nil, nil, validation.ErrorCode{
			Status: http.StatusNoContent,
		}
	}
	return obj, buffer, nil
}

var TypeGuidanceTable = map[schema.GroupVersionKind]map[string]string{
	schema.GroupVersionKind{Group: "management.cattle.io", Version: "v3", Kind: "Cluster"}: {
		"status.allocatable.cpuRaw":    "REAL",
		"status.allocatable.memoryRaw": "REAL",
		"status.allocatable.pods":      "INT",
		"status.requested.cpuRaw":      "REAL",
		"status.requested.memoryRaw":   "REAL",
		"status.requested.pods":        "INT",
	},
	schema.GroupVersionKind{Group: "provisioning.cattle.io", Version: "v1", Kind: "Cluster"}: {
		"status.allocatable.cpuRaw":    "REAL",
		"status.allocatable.memoryRaw": "REAL",
		"status.allocatable.pods":      "INT",
		"status.requested.cpuRaw":      "REAL",
		"status.requested.memoryRaw":   "REAL",
		"status.requested.pods":        "INT",
	},
	schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"}: {
		"metadata.fields[2]": "INT", // name: Data
	},
	schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ServiceAccount"}: {
		"metadata.fields[1]": "INT", // name: Secrets
	},
	schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}: {
		"metadata.fields[1]": "INT", // name: Data
	},
}

func getTypeGuidance(cols []common.ColumnDefinition, gvk schema.GroupVersionKind) map[string]string {
	guidance := make(map[string]string)
	ptn := regexp.MustCompile(`(?i)\bnumber of\b`)
	for _, col := range cols {
		td := col.TableColumnDefinition
		// These come from k8s.io/kubernetes/pkg/printers/internalversion
		// Some 'number of' fields are declared to be string, but we want to
		// sort those numbers numerically (like the POD # of a pod)
		colType := td.Type
		// Strip the parts off separately in case there's no '$' at the start
		trimmedField := strings.TrimPrefix(col.Field, "$")
		trimmedField = strings.TrimPrefix(trimmedField, ".")
		if colType == "integer" || colType == "boolean" || ptn.MatchString(td.Description) {
			//TODO: What do "REAL" (float) types look like?
			colType = "INT"
		}
		if colType != "string" {
			// Strip the parts off separately in case t
			guidance[trimmedField] = colType
		}
	}
	tg, ok := TypeGuidanceTable[gvk]
	if ok {
		for k, v := range tg {
			guidance[k] = v
		}
	}
	return guidance
}

// ListByPartitions returns:
//   - an unstructured list of resources belonging to any of the specified partitions
//   - the total number of resources (returned list might be a subset depending on pagination options in apiOp)
//   - a summary object, containing the possible values for each field specified in a summary= subquery
//   - a continue token, if there are more pages after the returned one
//   - an error instead of all of the above if anything went wrong
func (s *Store) ListByPartitions(apiOp *types.APIRequest, apiSchema *types.APISchema, partitions []partition.Partition) (list *unstructured.UnstructuredList, total int, summary *types.APISummary, continueToken string, err error) {
	ctx, cancel := context.WithCancel(apiOp.Context())
	defer cancel()

	inf, doneFn, err := s.cacheForWithDeps(ctx, apiOp, apiSchema)
	if err != nil {
		return
	}
	defer doneFn()

	gvk := attributes.GVK(apiSchema)
	var opts sqltypes.ListOptions
	opts, err = listprocessor.ParseQuery(apiOp, gvk.Kind)
	if err != nil {
		var apiError *apierror.APIError
		if errors.As(err, &apiError) {
			if apiError.Code.Status == http.StatusNoContent {
				list = &unstructured.UnstructuredList{}
				resourceVersion := inf.ByOptionsLister.GetLatestResourceVersion()
				if len(resourceVersion) > 0 {
					list.SetResourceVersion(resourceVersion[0])
				}
				return
			}
		}
		return
	}

	if gvk.Group == "ext.cattle.io" && (gvk.Kind == "Token" || gvk.Kind == "Kubeconfig") {
		accessSet := accesscontrol.AccessSetFromAPIRequest(apiOp)
		// See https://github.com/rancher/rancher/blob/7266e5e624f0d610c76ab0af33e30f5b72e11f61/pkg/ext/stores/tokens/tokens.go#L1186C2-L1195C3
		// for similar code on how we determine if a user is admin
		if accessSet == nil || !accessSet.Grants("list", schema.GroupResource{
			Resource: "*",
		}, "", "") {
			user, ok := request.UserFrom(apiOp.Request.Context())
			if !ok {
				err = apierror.NewAPIError(validation.MissingRequired, "failed to get user info from the request.Context object")
				return
			}
			opts.Filters = append(opts.Filters, sqltypes.OrFilter{
				Filters: []sqltypes.Filter{
					{
						Field:   []string{"metadata", "labels", "cattle.io/user-id"},
						Matches: []string{user.GetName()},
						Op:      sqltypes.Eq,
					},
				},
			})
		}
	}

	list, total, summary, continueToken, err = inf.ListByOptions(apiOp.Context(), &opts, partitions, apiOp.Namespace)
	if err != nil {
		if errors.Is(err, informer.ErrInvalidColumn) {
			err = apierror.NewAPIError(validation.InvalidBodyContent, err.Error())
		} else if errors.Is(err, informer.ErrUnknownRevision) {
			err = apierror.NewAPIError(validation.ErrorCode{Code: err.Error(), Status: http.StatusBadRequest}, err.Error())
		} else {
			err = fmt.Errorf("listbyoptions %v: %w", gvk, err)
		}
	}

	return
}

// WatchByPartitions returns a channel of events for a list or resource belonging to any of the specified partitions
func (s *Store) WatchByPartitions(apiOp *types.APIRequest, schema *types.APISchema, wr types.WatchRequest, partitions []partition.Partition) (chan watch.Event, error) {
	ctx, cancel := context.WithCancel(apiOp.Context())
	apiOp = apiOp.Clone().WithContext(ctx)

	eg := errgroup.Group{}

	result := make(chan watch.Event)

	for _, partition := range partitions {
		p := partition
		eg.Go(func() error {
			defer cancel()
			c, err := s.watchByPartition(p, apiOp, schema, wr)

			if err != nil {
				return err
			}
			for i := range c {
				result <- i
			}
			return nil
		})
	}

	go func() {
		defer close(result)
		<-ctx.Done()
		eg.Wait()
		cancel()
	}()

	return result, nil
}

// watchByPartition returns a channel of events for a list or resource belonging to a specified partition
func (s *Store) watchByPartition(partition partition.Partition, apiOp *types.APIRequest, schema *types.APISchema, wr types.WatchRequest) (chan watch.Event, error) {
	if partition.Passthrough {
		return s.Watch(apiOp, schema, wr)
	}

	apiOp.Namespace = partition.Namespace
	if partition.All {
		return s.Watch(apiOp, schema, wr)
	}
	return s.WatchNames(apiOp, schema, wr, partition.Names)
}

func (s *Store) cacheForWithDeps(ctx context.Context, apiOp *types.APIRequest, apiSchema *types.APISchema) (*factory.Cache, func(), error) {
	var doneCacheFns []func()
	doneCache := func() {
		length := len(doneCacheFns)
		for i := range length {
			fn := doneCacheFns[length-1-i]
			fn()
		}
	}

	gvk := attributes.GVK(apiSchema)
	// provisioning.cattle.io.clusters depends on information from management.cattle.io.clusters
	// so we must initialize this one as well
	if gvk == pcioClusterGvk {
		var mgmtSchema *types.APISchema
		if id := s.schemas.ByGVK(schema.GroupVersionKind{
			Group:   "management.cattle.io",
			Version: "v3",
			Kind:    "Cluster",
		}); id != "" {
			mgmtSchema = s.schemas.Schema(id)
		}

		if mgmtSchema == nil {
			return nil, nil, fmt.Errorf("management cluster schema not found")
		}

		mgmtClusterInf, err := s.cacheFor(ctx, nil, mgmtSchema)
		if err != nil {
			return nil, nil, err
		}
		doneCacheFns = append(doneCacheFns, func() {
			s.cacheFactory.DoneWithCache(mgmtClusterInf)
		})
	}

	inf, err := s.cacheFor(ctx, apiOp, apiSchema)
	if err != nil {
		doneCache()
		return nil, nil, err
	}
	doneCacheFns = append(doneCacheFns, func() {
		s.cacheFactory.DoneWithCache(inf)
	})

	return inf, doneCache, nil
}

func (s *Store) cacheFor(ctx context.Context, apiOp *types.APIRequest, apiSchema *types.APISchema) (*factory.Cache, error) {
	// warnings from inside the informer are discarded
	buffer := WarningBuffer{}
	client, err := s.clientGetter.TableAdminClient(apiOp, apiSchema, "", &buffer)
	if err != nil {
		return nil, err
	}

	gvk := attributes.GVK(apiSchema)
	//TODO: All this field information is only needed when `s.cf.CacheFor` needs to build the tables.
	// We should instead pass in a function to return the needed field info, rather than calculate it every time.
	fields, cols, typeGuidance := getFieldAndColInfo(apiSchema, gvk)
	fields = append(fields, getFieldForGVK(gvk)...)

	transformFunc := s.transformBuilder.GetTransformFunc(gvk, cols, attributes.IsCRD(apiSchema), attributes.CRDJSONPathParsers(apiSchema))
	tableClient := &tablelistconvert.Client{ResourceInterface: client}
	ns := attributes.Namespaced(apiSchema)
	inf, err := s.cacheFactory.CacheFor(ctx, fields, externalGVKDependencies[gvk], selfGVKDependencies[gvk], transformFunc, tableClient, gvk, typeGuidance, ns, controllerschema.IsListWatchable(apiSchema))
	if err != nil {
		return nil, fmt.Errorf("cachefor %v: %w", gvk, err)
	}
	return inf, nil
}
