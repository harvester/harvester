package rancher

import (
	"fmt"
	"reflect"

	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/utils/ptr"
)

// reconcileIngressResources reconciles DaemonSet resources and watchs until the rke2-traefik DS is found
// when one is found we check if an update of rancher-expose ingress object is needed
// this logic is handled as a standalone handler specifically for the upgade path scenario
// as the traefik DS can take a few mins after the
// node boots up by which time the harvester pod may have initialised
func (h *Handler) reconcileIngressResources(_ string, ds *appsv1.DaemonSet) (*appsv1.DaemonSet, error) {
	if ds == nil || ds.DeletionTimestamp != nil {
		return ds, nil
	}

	if ds.Name == util.Rke2TraefikAppName && ds.Namespace == util.KubeSystemNamespace {
		logrus.Info("found traefik ds, checking if ingress object needs an update")
		// traefik exists.. lets verify ingress and update settings
		ingressObj, err := h.ingresses.Get(util.CattleSystemNamespaceName, util.RancherExposeIngressName, metav1.GetOptions{})
		if err != nil {
			return ds, fmt.Errorf("error looking up ingress object %s: %w", util.RancherExposeIngressName, err)
		}

		var rerunExpose bool
		// update ingressClassName to traefik if needed
		if ingressObj.Spec.IngressClassName == nil || *ingressObj.Spec.IngressClassName != util.TraefikIngressClassName {
			ingressObj.Spec.IngressClassName = ptr.To(util.TraefikIngressClassName)
			if _, err := h.ingresses.Update(ingressObj); err != nil {
				return ds, fmt.Errorf("error updating ingress class on rancher-expose ingress: %w", err)
			}
			rerunExpose = true
		}

		// re-run registerExposeService may be needed since in HA nodes during upgrade traefik / nginx will be flapping until all controlplane nodes
		// get the update as part of the OS update
		svc, err := h.Services.Get(util.KubeSystemNamespace, traefikServiceName, v1.GetOptions{})
		if err != nil {
			return ds, fmt.Errorf("error fetching traefik service while attempting to update annotations: %w", err)
		}

		if doesTraefikServiceNeedUpdate(svc) {
			logrus.Info("traefik service needs update for kubevip annotations")
			rerunExpose = true
		}

		if rerunExpose {
			if err := h.registerExposeService(); err != nil {
				return ds, fmt.Errorf("error reconciling rancher expose service: %w", err)
			}
		}

		// restMapping will be successful when the CRD exists
		restMapping, err := findGVR(util.TraefikTLSStoreGVK, h.RestConfig)
		if err != nil {
			return ds, fmt.Errorf("error finding gvr for traefik tlsstore: %w", err)
		}
		_, err = h.DynamicClient.Resource(restMapping.Resource).Namespace(util.CattleSystemNamespaceName).Get(h.ctx, util.DefaultTraefikTLSStoreName, metav1.GetOptions{})
		if err == nil {
			return ds, nil
		}

		if apierrors.IsNotFound(err) {
			logrus.Infof("default traefik tlsstore not found, triggering update of tlscertificate setting to re-apply default tlsstore with new cert")
			if err := h.triggerTLSCertificateSettingUpdate(); err != nil {
				return ds, fmt.Errorf("error triggering tlscertificate setting update: %w", err)
			}
		}
		return ds, err
	}
	return ds, nil
}

func (h *Handler) triggerTLSCertificateSettingUpdate() error {
	tlsCertificateSetting, err := h.SettingCache.Get(settings.SSLCertificatesSettingName)
	if err != nil {
		return fmt.Errorf("error looking up ssl certificate setting: %w", err)
	}

	tlsCertificateSettingCopy := tlsCertificateSetting.DeepCopy()
	// update the setting value with the same value to trigger the change handler
	delete(tlsCertificateSettingCopy.Annotations, util.AnnotationHash)
	tlsCertificateSettingCopy.Annotations[util.AnnotationRancherControllerReconciled] = "true"
	if !reflect.DeepEqual(tlsCertificateSettingCopy, tlsCertificateSetting) {
		if _, err := h.SettingClient.Update(tlsCertificateSettingCopy); err != nil {
			return fmt.Errorf("error removing annotation hash from ssl certificate setting to trigger tls store update: %w", err)
		}
	}
	return nil
}

// lookup TLSStore using dynamic client
// since CRD will be created in upgrade path after the dynamic client
// has been initialised we need to use the rest mapper to lookup objects
func findGVR(gvk schema.GroupVersionKind, cfg *rest.Config) (*meta.RESTMapping, error) {
	// DiscoveryClient queries API server about the resources
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, err
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	newMapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, err
	}
	return newMapping, nil
}

func doesTraefikServiceNeedUpdate(svc *corev1.Service) bool {
	if svc.Annotations == nil {
		return true
	}

	val, ok := svc.Annotations[keyKubevipIgnoreServiceSecurity]
	if !ok || val != trueStr {
		return true
	}

	return false
}
