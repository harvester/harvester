package setting

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"time"

	provisioningv1 "github.com/rancher/rancher/pkg/apis/provisioning.cattle.io/v1"
	rkev1 "github.com/rancher/rancher/pkg/apis/rke.cattle.io/v1"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

const (
	defaultReconcilAutoRotateRKE2CertsSettingDuration = time.Hour * 24 // 1 day
)

func (h *Handler) syncAutoRotateRKE2Certs(setting *harvesterv1.Setting) error {
	logrus.WithFields(logrus.Fields{
		"name":  setting.Name,
		"value": setting.Value,
	}).Info("Processing setting")

	autoRotateRKE2Certs := &settings.AutoRotateRKE2Certs{}
	if err := json.Unmarshal([]byte(setting.Value), autoRotateRKE2Certs); err != nil {
		logrus.WithFields(logrus.Fields{
			"name":  setting.Name,
			"value": setting.Value,
		}).WithError(err).Error("failed to unmarshal setting value")
		return err
	}
	if !autoRotateRKE2Certs.Enable {
		return nil
	}

	kubernetesIP, err := h.getKubernetesIP()
	if err != nil {
		return err
	}
	if kubernetesIP == "" {
		err = fmt.Errorf("cluster ip is empty")
		logrus.WithFields(logrus.Fields{
			"name":              setting.Name,
			"service.namespace": metav1.NamespaceDefault,
			"service.name":      "kubernetes",
		}).WithError(err).Error("cluster ip is empty in the service")
		return err
	}

	earliestExpiringCert, err := h.getEarliestExpiringCert(fmt.Sprintf("%s:443", kubernetesIP))
	if err != nil {
		return err
	}
	if earliestExpiringCert == nil {
		logrus.WithFields(logrus.Fields{
			"name":              setting.Name,
			"service.namespace": metav1.NamespaceDefault,
			"service.name":      "kubernetes",
			"reconcileAfter":    defaultReconcilAutoRotateRKE2CertsSettingDuration,
		}).Warn("can't find certificate for cluster ip, reconcile setting again")
		h.settingController.EnqueueAfter(setting.Name, defaultReconcilAutoRotateRKE2CertsSettingDuration)
		return nil
	}
	logrus.WithField(
		"earliestExpiringCert", earliestExpiringCert,
	).Debug("earliest expiring cert for default/kubernetes ClusterIP")

	expiringInHours := time.Duration(autoRotateRKE2Certs.ExpiringInHours) * time.Hour
	if time.Now().Add(expiringInHours).After(earliestExpiringCert.NotAfter) {
		reconcileDuration, err := h.rotateRKE2Certs(setting)
		if err != nil {
			return err
		}

		h.settingController.EnqueueAfter(setting.Name, reconcileDuration)
		return nil
	}

	reconcileAfter := defaultReconcilAutoRotateRKE2CertsSettingDuration
	if earliestExpiringCert.NotAfter.Sub(time.Now().Add(expiringInHours)) < reconcileAfter {
		reconcileAfter = earliestExpiringCert.NotAfter.Sub(time.Now().Add(expiringInHours))
	}
	logrus.WithFields(logrus.Fields{
		"name":                          setting.Name,
		"expiringInHours":               expiringInHours,
		"earliestExpiringCert.notAfter": earliestExpiringCert.NotAfter,
		"reconcileAfter":                reconcileAfter,
	}).Info("RKE2 certificate is not expiring, reconcile setting again")
	h.settingController.EnqueueAfter(setting.Name, reconcileAfter)
	return nil
}

func (h *Handler) getKubernetesIP() (string, error) {
	svc, err := h.serviceCache.Get(metav1.NamespaceDefault, "kubernetes")
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"namespace": metav1.NamespaceDefault,
			"name":      "kubernetes",
		}).WithError(err).Error("serviceCache.Get")
		return "", err
	}

	return svc.Spec.ClusterIP, nil
}

func (h *Handler) getEarliestExpiringCert(addr string) (*x509.Certificate, error) {
	conf := &tls.Config{
		InsecureSkipVerify: true,
	}

	conn, err := tls.Dial("tcp", addr, conf)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"addr": addr,
			"conf": conf,
		}).WithError(err).Error("tls.Dial")
		return nil, err
	}
	defer conn.Close()

	var earliestExpiringCert *x509.Certificate
	certs := conn.ConnectionState().PeerCertificates
	for _, cert := range certs {
		if earliestExpiringCert == nil || earliestExpiringCert.NotAfter.After(cert.NotAfter) {
			earliestExpiringCert = cert
		}
	}

	return earliestExpiringCert, nil
}

func (h *Handler) rotateRKE2Certs(setting *harvesterv1.Setting) (time.Duration, error) {
	cluster, err := h.clusterCache.Get(util.FleetLocalNamespaceName, util.LocalClusterName)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"cluster.namespace": util.FleetLocalNamespaceName,
			"cluster.name":      util.LocalClusterName,
		}).WithError(err).Error("clusterCache.Get")
		return time.Duration(0), err
	}

	rkeControlPlane, err := h.rkeControlPlaneCache.Get(util.FleetLocalNamespaceName, util.LocalClusterName)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"rkeControlPlane.namespace": util.FleetLocalNamespaceName,
			"rkeControlPlane.name":      util.LocalClusterName,
		}).WithError(err).Error("rkeControlPlaneCache.Get")
		return time.Duration(0), err
	}

	isRKEControlPlaneReady := false
	for _, cond := range rkeControlPlane.Status.Conditions {
		if cond.Type == "Ready" && cond.Status == v1.ConditionTrue {
			isRKEControlPlaneReady = true
			break
		}
	}

	quickReconcilDuration := time.Second * 30
	if !isRKEControlPlaneReady {
		logrus.WithFields(logrus.Fields{
			"name":                      setting.Name,
			"rkeControlPlane.namespace": util.FleetLocalNamespaceName,
			"rkeControlPlane.name":      util.LocalClusterName,
			"reconcileAfter":            quickReconcilDuration,
		}).Info("rkecontrolplane is not ready, reconcile setting again")
		return quickReconcilDuration, nil
	}

	clusterCopy := cluster.DeepCopy()
	if clusterCopy.Spec.RKEConfig == nil {
		clusterCopy.Spec.RKEConfig = &provisioningv1.RKEConfig{}
	}
	if clusterCopy.Spec.RKEConfig.RotateCertificates == nil {
		clusterCopy.Spec.RKEConfig.RotateCertificates = &rkev1.RotateCertificates{}
	}

	clusterRotateCertificatesGeneration := clusterCopy.Spec.RKEConfig.RotateCertificates.Generation
	rkeControlPlaneRotateCertificatesGeneration := int64(0)
	if rkeControlPlane.Spec.RotateCertificates != nil {
		rkeControlPlaneRotateCertificatesGeneration = rkeControlPlane.Spec.RotateCertificates.Generation
	}
	if clusterRotateCertificatesGeneration != rkeControlPlaneRotateCertificatesGeneration ||
		rkeControlPlaneRotateCertificatesGeneration != rkeControlPlane.Status.CertificateRotationGeneration {
		logrus.WithFields(logrus.Fields{
			"name":              setting.Name,
			"cluster.namespace": util.FleetLocalNamespaceName,
			"cluster.name":      util.LocalClusterName,
			"reconcileAfter":    quickReconcilDuration,
		}).Info("rotateCertificates.Generation is not synced between cluster and rkeControlPlane, reconcile setting again")
		return quickReconcilDuration, nil
	}

	clusterCopy.Spec.RKEConfig.RotateCertificates.Generation++
	if _, err := h.clusters.Update(clusterCopy); err != nil {
		logrus.WithFields(logrus.Fields{
			"namespace": util.FleetLocalNamespaceName,
			"name":      util.LocalClusterName,
		}).WithError(err).Error("clusters.Update")
		return time.Duration(0), err
	}

	// if users don't udpate the setting, the controller can't be triggered
	// so we still need to enqueue the setting again to check certs after rotate
	return defaultReconcilAutoRotateRKE2CertsSettingDuration, nil
}
