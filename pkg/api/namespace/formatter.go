package namespace

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"

	"github.com/gorilla/mux"
	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/wrangler/v3/pkg/schemas/validation"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/kubernetes"

	apiutil "github.com/harvester/harvester/pkg/api/util"
	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

const (
	updateResourceQuotaAction = "updateResourceQuota"
	deleteResourceQuotaAction = "deleteResourceQuota"
)

type Handler struct {
	resourceQuotaClient ctlharvesterv1.ResourceQuotaClient
	clientSet           kubernetes.Clientset

	ctx context.Context
}

type nsformatter struct {
	clientSet kubernetes.Clientset
}

func (nf *nsformatter) formatter(request *types.APIRequest, resource *types.RawResource) {
	resource.Actions = make(map[string]string, 1)
	if request.AccessControl.CanUpdate(request, resource.APIObject, resource.Schema) != nil {
		return
	}

	if ok, err := apiutil.CanUpdateResourceQuota(nf.clientSet, request.Namespace, request.GetUser()); err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"namespace": request.Namespace,
			"user":      request.GetUser(),
		}).Error("Failed to check update resource quota")
	} else if ok {
		resource.AddAction(request, updateResourceQuotaAction)
		resource.AddAction(request, deleteResourceQuotaAction)
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if err := h.do(rw, req); err != nil {
		status := http.StatusInternalServerError
		if e, ok := err.(*apierror.APIError); ok {
			status = e.Code.Status
		}
		rw.WriteHeader(status)
		_, _ = rw.Write([]byte(err.Error()))
		return
	}
	rw.WriteHeader(http.StatusOK)
}

func (h *Handler) do(_ http.ResponseWriter, req *http.Request) error {
	vars := util.EncodeVars(mux.Vars(req))
	action := vars["action"]
	namespace := vars["name"] // since this is namespace api, so name is namespace here

	user, ok := request.UserFrom(req.Context())
	if !ok {
		return apierror.NewAPIError(validation.Unauthorized, "failed to get user from request")
	}

	switch action {
	case updateResourceQuotaAction:
		if ok, err := apiutil.CanUpdateResourceQuota(h.clientSet, namespace, user.GetName()); err != nil {
			return apierror.NewAPIError(validation.ServerError, fmt.Sprintf("Failed to check permission: %v", err))
		} else if !ok {
			return apierror.NewAPIError(validation.PermissionDenied, "User does not have permission to update resource quota")
		}

		var updateResourceQuotaInput UpdateResourceQuotaInput
		if err := json.NewDecoder(req.Body).Decode(&updateResourceQuotaInput); err != nil {
			return apierror.NewAPIError(validation.InvalidBodyContent, fmt.Sprintf("Failed to decode request body: %v ", err))
		}
		return h.updateResourceQuota(namespace, updateResourceQuotaInput)
	case deleteResourceQuotaAction:
		if ok, err := apiutil.CanUpdateResourceQuota(h.clientSet, namespace, user.GetName()); err != nil {
			return apierror.NewAPIError(validation.ServerError, fmt.Sprintf("Failed to check permission: %v", err))
		} else if !ok {
			return apierror.NewAPIError(validation.PermissionDenied, "User does not have permission to update resource quota")
		}
		return h.deleteResourceQuota(namespace)
	default:
		return apierror.NewAPIError(validation.InvalidAction, "Unsupported action")
	}
}

func (h *Handler) updateResourceQuota(namespace string, updateResourceQuotaInput UpdateResourceQuotaInput) error {
	totalSnapshotSizeQuotaQuantity, err := resource.ParseQuantity(updateResourceQuotaInput.TotalSnapshotSizeQuota)
	if err != nil {
		return apierror.NewAPIError(validation.InvalidBodyContent, fmt.Sprintf("Failed to parse totalSnapshotSizeQuota: %v", err))
	}
	totalSnapshotSizeQuotaInt64, ok := totalSnapshotSizeQuotaQuantity.AsInt64()
	if !ok {
		return apierror.NewAPIError(validation.InvalidBodyContent, "Failed to parse totalSnapshotSizeQuota as int64")
	}

	if namespace == "" {
		namespace = metav1.NamespaceDefault
	}
	resourceQuota, err := h.resourceQuotaClient.Get(namespace, util.DefaultResourceQuotaName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			resourceQuota = &harvesterv1.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      util.DefaultResourceQuotaName,
				},
				Spec: harvesterv1.ResourceQuotaSpec{
					SnapshotLimit: harvesterv1.SnapshotLimit{
						NamespaceTotalSnapshotSizeQuota: totalSnapshotSizeQuotaInt64,
					},
				},
			}
			_, err = h.resourceQuotaClient.Create(resourceQuota)
			return err
		}
		return err
	}

	resourceQuotaCopy := resourceQuota.DeepCopy()
	resourceQuotaCopy.Spec.SnapshotLimit.NamespaceTotalSnapshotSizeQuota = totalSnapshotSizeQuotaInt64
	if !reflect.DeepEqual(resourceQuota, resourceQuotaCopy) {
		_, err = h.resourceQuotaClient.Update(resourceQuotaCopy)
		return err
	}
	return nil
}

func (h *Handler) deleteResourceQuota(namespace string) error {
	resourceQuota, err := h.resourceQuotaClient.Get(namespace, util.DefaultResourceQuotaName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	resourceQuotaCopy := resourceQuota.DeepCopy()
	resourceQuotaCopy.Spec.SnapshotLimit.NamespaceTotalSnapshotSizeQuota = 0
	if !reflect.DeepEqual(resourceQuota, resourceQuotaCopy) {
		_, err = h.resourceQuotaClient.Update(resourceQuotaCopy)
		return err
	}
	return nil
}
