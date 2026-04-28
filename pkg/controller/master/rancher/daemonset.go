package rancher

import (
	"fmt"
	"reflect"

	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

		// update ingressClassName to traefik if needed
		if ingressObj.Spec.IngressClassName == nil || *ingressObj.Spec.IngressClassName != util.TraefikIngressClassName {
			ingressObj.Spec.IngressClassName = ptr.To(util.TraefikIngressClassName)
			if _, err := h.ingresses.Update(ingressObj); err != nil {
				return ds, fmt.Errorf("error updating ingress class on rancher-expose ingress: %w", err)
			}

			// re-run registerExposeService as its only run on boot of controller
			if err := h.registerExposeService(); err != nil {
				return ds, fmt.Errorf("error reconciling rancher expose service: %w", err)
			}

		}

		// if tls-ingress secret does not exist in kube-system namespace then remove annotation hash from ssl-certificates setting
		// to force reconcile and recreation of secrets
		// remove hash annotations from ssl related settings to ensure

		_, err = h.SecretCache.Get(util.KubeSystemNamespace, util.TraefikIngressSecret)
		if err == nil {
			// secret exists, no further action is needed
			return ds, nil
		}

		if apierrors.IsNotFound(err) {
			// patch settings to ensure correct tls settings are setup for node
			for _, settingName := range []string{settings.SSLCertificatesSettingName, settings.SSLParametersName} {
				settingObj, err := h.SettingCache.Get(settingName)
				if err != nil {
					return ds, fmt.Errorf("error fetching setting %s: %w", settingName, err)
				}
				settingObjCopy := settingObj.DeepCopy()
				delete(settingObjCopy.Annotations, util.AnnotationHash)
				logrus.Infof("patching setting %s to remove annotation hash to force reconcile of ssl settings", settingName)
				if !reflect.DeepEqual(settingObj, settingObjCopy) {
					if _, err := h.SettingClient.Update(settingObjCopy); err != nil {
						return ds, fmt.Errorf("error clearing setting annotation hash for setting %s: %w", settingName, err)
					}
				}
			}
		} else {
			// unexpected error, requeue and try again
			return ds, err
		}
	}
	return ds, nil
}
