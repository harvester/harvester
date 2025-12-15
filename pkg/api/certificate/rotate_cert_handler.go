package certificate

import (
	"context"

	apierror "github.com/rancher/apiserver/pkg/apierror"
	provisioningv1 "github.com/rancher/rancher/pkg/apis/provisioning.cattle.io/v1"
	rkev1 "github.com/rancher/rancher/pkg/apis/rke.cattle.io/v1"
	ctlprovisioningv1 "github.com/rancher/rancher/pkg/generated/controllers/provisioning.cattle.io/v1"
	ctlrkev1 "github.com/rancher/rancher/pkg/generated/controllers/rke.cattle.io/v1"
	"github.com/rancher/wrangler/v3/pkg/schemas/validation"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"

	"github.com/harvester/harvester/pkg/config"
	harvesterServer "github.com/harvester/harvester/pkg/server/http"
	"github.com/harvester/harvester/pkg/util"
)

type RotateCertHandler struct {
	context              context.Context
	clusterCache         ctlprovisioningv1.ClusterCache
	clusters             ctlprovisioningv1.ClusterClient
	rkeControlPlaneCache ctlrkev1.RKEControlPlaneCache
}

func NewRotateCertHandler(scaled *config.Scaled) *RotateCertHandler {
	return &RotateCertHandler{
		context:      scaled.Ctx,
		clusterCache: scaled.Management.ProvisioningFactory.Provisioning().V1().Cluster().Cache(),
		clusters:     scaled.Management.ProvisioningFactory.Provisioning().V1().Cluster(),
	}
}

func (h *RotateCertHandler) Do(ctx *harvesterServer.Ctx) (harvesterServer.ResponseBody, error) {
	cluster, err := h.clusterCache.Get(util.FleetLocalNamespaceName, util.LocalClusterName)
	if err != nil {
		logrus.WithError(err).Error("clusterCache.Get")
		return nil, err
	}

	rkeControlPlane, err := h.rkeControlPlaneCache.Get(util.FleetLocalNamespaceName, util.LocalClusterName)
	if err != nil {
		logrus.WithError(err).Error("rkeControlPlaneCache.Get")
		return nil, err
	}

	isRKEControlPlaneReady := false
	for _, cond := range rkeControlPlane.Status.Conditions {
		if cond.Type == "Ready" && cond.Status == corev1.ConditionTrue {
			isRKEControlPlaneReady = true
			break
		}
	}

	if !isRKEControlPlaneReady {
		return nil, apierror.NewAPIError(validation.ErrorCode{
			Code:   "RKEControlPlaneNotReady",
			Status: 400,
		}, "rkecontrolplane is not ready")
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
		return nil, apierror.NewAPIError(validation.ErrorCode{
			Code:   "RotationAlreadyInProgress",
			Status: 400,
		}, "certificate rotation is already in progress")
	}

	clusterCopy.Spec.RKEConfig.RotateCertificates.Generation++
	if _, err := h.clusters.Update(clusterCopy); err != nil {
		return nil, err
	}

	ctx.SetStatusOK()
	return nil, nil
}
