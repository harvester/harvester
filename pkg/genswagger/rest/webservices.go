package rest

import (
	"fmt"

	"github.com/emicklei/go-restful/v3"
	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	virtv1 "kubevirt.io/api/core/v1"

	networkv1beta1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

func AggregatedWebServices() []*restful.WebService {
	harvesterv1beta1API := NewGroupVersionWebService(v1beta1.SchemeGroupVersion)
	NewGenericResource(harvesterv1beta1API, "virtualmachinebackups", &v1beta1.VirtualMachineBackup{}, "VirtualMachineBackup", &v1beta1.VirtualMachineBackupList{}, true, defaultActions)
	NewGenericResource(harvesterv1beta1API, "virtualmachinerestores", &v1beta1.VirtualMachineRestore{}, "VirtualMachineRestore", &v1beta1.VirtualMachineRestoreList{}, true, defaultActions)
	NewGenericResource(harvesterv1beta1API, "virtualmachineimages", &v1beta1.VirtualMachineImage{}, "VirtualMachineImage", &v1beta1.VirtualMachineImageList{}, true, defaultActions)
	NewGenericResource(harvesterv1beta1API, "virtualmachinetemplates", &v1beta1.VirtualMachineTemplate{}, "VirtualMachineTemplate", &v1beta1.VirtualMachineTemplateList{}, true, defaultActions)
	NewGenericResource(harvesterv1beta1API, "virtualmachinetemplateversions", &v1beta1.VirtualMachineTemplateVersion{}, "VirtualMachineTemplateVersion", &v1beta1.VirtualMachineTemplateVersionList{}, true, defaultActions)
	NewGenericResource(harvesterv1beta1API, "keypairs", &v1beta1.KeyPair{}, "KeyPair", &v1beta1.KeyPairList{}, true, defaultActions)
	NewGenericResource(harvesterv1beta1API, "upgrades", &v1beta1.Upgrade{}, "Upgrade", &v1beta1.UpgradeList{}, true, defaultActions)
	NewGenericResource(harvesterv1beta1API, "supportbundles", &v1beta1.SupportBundle{}, "SupportBundle", &v1beta1.SupportBundleList{}, true, defaultActions)

	harvesterNetworkv1beta1API := NewGroupVersionWebService(networkv1beta1.SchemeGroupVersion)
	NewGenericResource(harvesterNetworkv1beta1API, "clusternetworks", &networkv1beta1.ClusterNetwork{}, "ClusterNetwork", &networkv1beta1.ClusterNetworkList{}, false, defaultActions)

	// core
	corev1API := NewGroupVersionWebService(corev1.SchemeGroupVersion)
	NewGenericResource(corev1API, "persistentvolumeclaims", &corev1.PersistentVolumeClaim{}, "PersistentVolumeClaim", &corev1.PersistentVolumeClaimList{}, true, defaultActions)

	// kubevirt
	virtv1API := NewGroupVersionWebService(virtv1.SchemeGroupVersion)
	NewGenericResource(virtv1API, "virtualmachineinstances", &virtv1.VirtualMachineInstance{}, "VirtualMachineInstance", &virtv1.VirtualMachineInstanceList{}, true, defaultGetActions)
	NewGenericResource(virtv1API, "virtualmachines", &virtv1.VirtualMachine{}, "VirtualMachine", &virtv1.VirtualMachineList{}, true, defaultActions)
	NewGenericResource(virtv1API, "virtualmachineinstancemigrations", &virtv1.VirtualMachineInstanceMigration{}, "VirtualMachineInstanceMigration", &virtv1.VirtualMachineInstanceMigrationList{}, true, defaultActions)

	// multus
	cniv1API := NewGroupVersionWebService(cniv1.SchemeGroupVersion)
	NewGenericResource(cniv1API, "network-attachment-definitions", &cniv1.NetworkAttachmentDefinition{}, "NetworkAttachmentDefinition", &cniv1.NetworkAttachmentDefinitionList{}, true, defaultActions)

	return []*restful.WebService{harvesterv1beta1API, harvesterNetworkv1beta1API, corev1API, virtv1API, cniv1API}
}

func NewGroupVersionWebService(gv schema.GroupVersion) *restful.WebService {
	ws := new(restful.WebService)
	ws.Doc("The Harvester API.")
	ws.Path(GroupVersionBasePath(gv))

	return ws
}

func addCollectionParams(builder *restful.RouteBuilder, ws *restful.WebService) *restful.RouteBuilder {
	return builder.Param(continueParam(ws)).
		Param(fieldSelectorParam(ws)).
		Param(includeUninitializedParam(ws)).
		Param(labelSelectorParam(ws)).
		Param(limitParam(ws)).
		Param(resourceVersionParam(ws)).
		Param(timeoutSecondsParam(ws)).
		Param(watchParam(ws))
}

func NameParam(ws *restful.WebService) *restful.Parameter {
	return ws.PathParameter("name", "Name of the resource").
		Required(true).
		Pattern("[a-z0-9][a-z0-9\\-]*")
}

func NamespaceParam(ws *restful.WebService) *restful.Parameter {
	return ws.PathParameter("namespace", "Object name and auth scope, such as for teams and projects").
		Required(true).
		Pattern("[a-z0-9][a-z0-9\\-]*")
}

func labelSelectorParam(ws *restful.WebService) *restful.Parameter {
	return ws.QueryParameter("labelSelector", "A selector to restrict the list of returned objects by their labels. Defaults to everything")
}

func fieldSelectorParam(ws *restful.WebService) *restful.Parameter {
	return ws.QueryParameter("fieldSelector", "A selector to restrict the list of returned objects by their fields. Defaults to everything.")
}

func resourceVersionParam(ws *restful.WebService) *restful.Parameter {
	return ws.QueryParameter("resourceVersion", "When specified with a watch call, shows changes that occur after that particular version of a resource. Defaults to changes from the beginning of history.")
}

func timeoutSecondsParam(ws *restful.WebService) *restful.Parameter {
	return ws.QueryParameter("timeoutSeconds", "TimeoutSeconds for the list/watch call.").DataType("integer")
}

func includeUninitializedParam(ws *restful.WebService) *restful.Parameter {
	return ws.QueryParameter("includeUninitialized", "If true, partially initialized resources are included in the response.").DataType("boolean")
}

func watchParam(ws *restful.WebService) *restful.Parameter {
	return ws.QueryParameter("watch", "Watch for changes to the described resources and return them as a stream of add, update, and remove notifications. Specify resourceVersion.").DataType("boolean")
}

func limitParam(ws *restful.WebService) *restful.Parameter {
	return ws.QueryParameter("limit", "limit is a maximum number of responses to return for a list call. If more items exist, the server will set the `continue` field on the list metadata to a value that can be used with the same initial query to retrieve the next set of results. Setting a limit may return fewer than the requested amount of items (up to zero items) in the event all requested objects are filtered out and clients should only use the presence of the continue field to determine whether more results are available. Servers may choose not to support the limit argument and will return all of the available results. If limit is specified and the continue field is empty, clients may assume that no more results are available. This field is not supported if watch is true.\n\nThe server guarantees that the objects returned when using continue will be identical to issuing a single list call without a limit - that is, no objects created, modified, or deleted after the first request is issued will be included in any subsequent continued requests. This is sometimes referred to as a consistent snapshot, and ensures that a client that is using limit to receive smaller chunks of a very large result can ensure they see all possible objects. If objects are updated during a chunked list the version of the object that was present at the time the first list result was calculated is returned.").DataType("integer")
}

func continueParam(ws *restful.WebService) *restful.Parameter {
	return ws.QueryParameter("continue", "The continue option should be set when retrieving more results from the server. Since this value is server defined, clients may only use the continue value from a previous query result with identical query parameters (except for the value of continue) and the server may reject a continue value it does not recognize. If the specified continue value is no longer valid whether due to expiration (generally five to fifteen minutes) or a configuration change on the server the server will respond with a 410 ResourceExpired error indicating the client must restart their list without the continue field. This field is not supported when watch is true. Clients may start a watch from the last resourceVersion value returned by the server and not miss any modifications.")
}

func exactParam(ws *restful.WebService) *restful.Parameter {
	return ws.QueryParameter("exact", "Should the export be exact. Exact export maintains cluster-specific fields like 'Namespace'.").DataType("boolean")
}

func exportParam(ws *restful.WebService) *restful.Parameter {
	return ws.QueryParameter("export", "Should this value be exported. Export strips fields that a user can not specify.").DataType("boolean")
}

func gracePeriodSecondsParam(ws *restful.WebService) *restful.Parameter {
	return ws.QueryParameter("gracePeriodSeconds", "The duration in seconds before the object should be deleted. Value must be non-negative integer. The value zero indicates delete immediately. If this value is nil, the default grace period for the specified type will be used. Defaults to a per object value if not specified. zero means delete immediately.").DataType("integer")
}

func orphanDependentsParam(ws *restful.WebService) *restful.Parameter {
	return ws.QueryParameter("orphanDependents", "Deprecated: please use the PropagationPolicy, this field will be deprecated in 1.7. Should the dependent objects be orphaned. If true/false, the \"orphan\" finalizer will be added to/removed from the object's finalizers list. Either this field or PropagationPolicy may be set, but not both.").DataType("boolean")
}

func propagationPolicyParam(ws *restful.WebService) *restful.Parameter {
	return ws.QueryParameter("propagationPolicy", "Whether and how garbage collection will be performed. Either this field or OrphanDependents may be set, but not both. The default policy is decided by the existing finalizer set in the metadata.finalizers and the resource-specific default policy. Acceptable values are: 'Orphan' - orphan the dependents; 'Background' - allow the garbage collector to delete the dependents in the background; 'Foreground' - a cascading policy that deletes all dependents in the foreground.")
}

func GroupVersionBasePath(gvr schema.GroupVersion) string {
	if gvr.Group == corev1.GroupName {
		return "/api/v1"
	}
	return fmt.Sprintf("/apis/%s/%s", gvr.Group, gvr.Version)
}

func ResourceBasePath(resource string, namespaced bool) string {
	if namespaced {
		return fmt.Sprintf("/namespaces/{namespace}/%s", resource)
	}
	return fmt.Sprintf("/%s", resource)
}

func ResourcePath(resource string, namespaced bool) string {
	if namespaced {
		return fmt.Sprintf("/namespaces/{namespace}/%s/{name}", resource)
	}
	return fmt.Sprintf("/%s/{name}", resource)
}

func Noop(*restful.Request, *restful.Response) {}
