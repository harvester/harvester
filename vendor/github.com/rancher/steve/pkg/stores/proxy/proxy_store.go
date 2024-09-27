// Package proxy implements the proxy store, which is responsible for interfacing directly with Kubernetes.
package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strconv"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/wrangler/v3/pkg/data"
	corecontrollers "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/v3/pkg/schemas/validation"
	"github.com/rancher/wrangler/v3/pkg/summary"

	"github.com/rancher/steve/pkg/accesscontrol"
	"github.com/rancher/steve/pkg/attributes"
	metricsStore "github.com/rancher/steve/pkg/stores/metrics"
	"github.com/rancher/steve/pkg/stores/partition"
)

const (
	watchTimeoutEnv      = "CATTLE_WATCH_TIMEOUT_SECONDS"
	errNamespaceRequired = "metadata.namespace is required"
)

var (
	lowerChars  = regexp.MustCompile("[a-z]+")
	paramScheme = runtime.NewScheme()
	paramCodec  = runtime.NewParameterCodec(paramScheme)
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

// Store implements partition.UnstructuredStore directly on top of kubernetes.
type Store struct {
	clientGetter ClientGetter
	notifier     RelationshipNotifier
}

// NewProxyStore returns a wrapped types.Store.
func NewProxyStore(clientGetter ClientGetter, notifier RelationshipNotifier, lookup accesscontrol.AccessSetLookup, namespaceCache corecontrollers.NamespaceCache) types.Store {
	return &ErrorStore{
		Store: &unformatterStore{
			Store: &WatchRefresh{
				Store: partition.NewStore(
					&rbacPartitioner{
						proxyStore: &Store{
							clientGetter: clientGetter,
							notifier:     notifier,
						},
					},
					lookup,
					namespaceCache,
				),
				asl: lookup,
			},
		},
	}
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

func tableToList(obj *unstructured.UnstructuredList) {
	if obj.Object["kind"] != "Table" ||
		(obj.Object["apiVersion"] != "meta.k8s.io/v1" &&
			obj.Object["apiVersion"] != "meta.k8s.io/v1beta1") {
		return
	}

	obj.Items = tableToObjects(obj.Object)
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

// ByNames filters a list of objects by an allowed set of names.
// In plain kubernetes, if a user has permission to 'list' or 'watch' a defined set of resource names,
// performing the list or watch will result in a Forbidden error, because the user does not have permission
// to list *all* resources.
// With this filter, the request can be performed successfully, and only the allowed resources will
// be returned in the list.
func (s *Store) ByNames(apiOp *types.APIRequest, schema *types.APISchema, names sets.String) (*unstructured.UnstructuredList, []types.Warning, error) {
	if apiOp.Namespace == "*" {
		// This happens when you grant namespaced objects with "get" or "list "by name in a clusterrolebinding.
		// We will treat this as an invalid situation instead of listing all objects in the cluster
		// and filtering by name.
		return &unstructured.UnstructuredList{}, nil, nil
	}
	buffer := WarningBuffer{}
	adminClient, err := s.clientGetter.TableAdminClient(apiOp, schema, apiOp.Namespace, &buffer)
	if err != nil {
		return nil, nil, err
	}

	objs, err := s.list(apiOp, schema, adminClient)
	if err != nil {
		return nil, nil, err
	}

	var filtered []unstructured.Unstructured
	for _, obj := range objs.Items {
		if names.Has(obj.GetName()) {
			filtered = append(filtered, obj)
		}
	}

	objs.Items = filtered
	return objs, buffer, nil
}

// List returns an unstructured list of resources.
func (s *Store) List(apiOp *types.APIRequest, schema *types.APISchema) (*unstructured.UnstructuredList, []types.Warning, error) {
	buffer := WarningBuffer{}
	client, err := s.clientGetter.TableClient(apiOp, schema, apiOp.Namespace, &buffer)
	if err != nil {
		return nil, nil, err
	}
	result, err := s.list(apiOp, schema, client)
	return result, buffer, err
}

func (s *Store) list(apiOp *types.APIRequest, schema *types.APISchema, client dynamic.ResourceInterface) (*unstructured.UnstructuredList, error) {
	opts := metav1.ListOptions{}
	if err := decodeParams(apiOp, &opts); err != nil {
		return nil, nil
	}

	k8sClient, _ := metricsStore.Wrap(client, nil)
	resultList, err := k8sClient.List(apiOp, opts)
	if err != nil {
		return nil, err
	}

	tableToList(resultList)

	return resultList, nil
}

func returnErr(err error, c chan watch.Event) {
	c <- watch.Event{
		Type: watch.Error,
		Object: &metav1.Status{
			Message: err.Error(),
		},
	}
}

func (s *Store) listAndWatch(apiOp *types.APIRequest, client dynamic.ResourceInterface, schema *types.APISchema, w types.WatchRequest, result chan watch.Event) {
	rev := w.Revision
	if rev == "-1" || rev == "0" {
		rev = ""
	}

	timeout := int64(60 * 30)
	timeoutSetting := os.Getenv(watchTimeoutEnv)
	if timeoutSetting != "" {
		userSetTimeout, err := strconv.Atoi(timeoutSetting)
		if err != nil {
			logrus.Debugf("could not parse %s environment variable, error: %v", watchTimeoutEnv, err)
		} else {
			timeout = int64(userSetTimeout)
		}
	}
	k8sClient, _ := metricsStore.Wrap(client, nil)
	watcher, err := k8sClient.Watch(apiOp, metav1.ListOptions{
		Watch:           true,
		TimeoutSeconds:  &timeout,
		ResourceVersion: rev,
		LabelSelector:   w.Selector,
	})
	if err != nil {
		returnErr(errors.Wrapf(err, "stopping watch for %s: %v", schema.ID, err), result)
		return
	}
	defer watcher.Stop()
	logrus.Debugf("opening watcher for %s", schema.ID)

	eg, ctx := errgroup.WithContext(apiOp.Context())

	go func() {
		<-ctx.Done()
		watcher.Stop()
	}()

	if s.notifier != nil {
		eg.Go(func() error {
			for rel := range s.notifier.OnInboundRelationshipChange(ctx, schema, apiOp.Namespace) {
				obj, _, err := s.byID(apiOp, schema, rel.Namespace, rel.Name)
				if err == nil {
					rowToObject(obj)
					result <- watch.Event{Type: watch.Modified, Object: obj}
				} else {
					returnErr(errors.Wrapf(err, "notifier watch error: %v", err), result)
				}
			}
			return fmt.Errorf("closed")
		})
	}

	eg.Go(func() error {
		for event := range watcher.ResultChan() {
			if event.Type == watch.Error {
				if status, ok := event.Object.(*metav1.Status); ok {
					returnErr(fmt.Errorf("event watch error: %s", status.Message), result)
				} else {
					logrus.Debugf("event watch error: could not decode event object %T", event.Object)
				}
				continue
			}
			if unstr, ok := event.Object.(*unstructured.Unstructured); ok {
				rowToObject(unstr)
			}
			result <- event
		}
		return fmt.Errorf("closed")
	})

	_ = eg.Wait()
	return
}

// WatchNames returns a channel of events filtered by an allowed set of names.
// In plain kubernetes, if a user has permission to 'list' or 'watch' a defined set of resource names,
// performing the list or watch will result in a Forbidden error, because the user does not have permission
// to list *all* resources.
// With this filter, the request can be performed successfully, and only the allowed resources will
// be returned in watch.
func (s *Store) WatchNames(apiOp *types.APIRequest, schema *types.APISchema, w types.WatchRequest, names sets.String) (chan watch.Event, error) {
	buffer := &WarningBuffer{}
	adminClient, err := s.clientGetter.TableAdminClientForWatch(apiOp, schema, apiOp.Namespace, buffer)
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
	client, err := s.clientGetter.TableClientForWatch(apiOp, schema, apiOp.Namespace, buffer)
	if err != nil {
		return nil, err
	}
	return s.watch(apiOp, schema, w, client)
}

func (s *Store) watch(apiOp *types.APIRequest, schema *types.APISchema, w types.WatchRequest, client dynamic.ResourceInterface) (chan watch.Event, error) {
	result := make(chan watch.Event)
	go func() {
		s.listAndWatch(apiOp, client, schema, w, result)
		logrus.Debugf("closing watcher for %s", schema.ID)
		close(result)
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
	k8sClient, err := metricsStore.Wrap(s.clientGetter.TableClient(apiOp, schema, namespace, &buffer))
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
	k8sClient, err := metricsStore.Wrap(s.clientGetter.TableClient(apiOp, schema, ns, &buffer))
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
		return nil, nil, fmt.Errorf("metadata.resourceVersion is required for update")
	}

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
	k8sClient, err := metricsStore.Wrap(s.clientGetter.TableClient(apiOp, schema, apiOp.Namespace, &buffer))
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
