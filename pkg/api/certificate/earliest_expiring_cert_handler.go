package certificate

import (
	"context"
	"time"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/harvester/harvester/pkg/config"
	harvesterServer "github.com/harvester/harvester/pkg/server/http"
	"github.com/harvester/harvester/pkg/util"
)

type EarliestExpiringCertHandler struct {
	context   context.Context
	nodeCache ctlcorev1.NodeCache
}

type EarliestExpiringCertResponse struct {
	NotAfter time.Time `json:"notAfter"`
}

func NewEarliestExpiringCertHandler(scaled *config.Scaled) *EarliestExpiringCertHandler {
	return &EarliestExpiringCertHandler{
		context:   scaled.Ctx,
		nodeCache: scaled.CoreFactory.Core().V1().Node().Cache(),
	}
}

func (h *EarliestExpiringCertHandler) Do(ctx *harvesterServer.Ctx) (harvesterServer.ResponseBody, error) {
	nodes, err := h.nodeCache.List(labels.Everything())
	if err != nil {
		logrus.WithError(err).Error("nodeCache.List")
		return nil, err
	}

	controlPlaneIps, witnessIps, workerIps := util.GetNodeIps(nodes)
	earliestExpiringCert, err := util.GetAddrsEarliestExpiringCert(controlPlaneIps, witnessIps, workerIps)
	if err != nil {
		return nil, err
	}

	resp := &EarliestExpiringCertResponse{}
	if earliestExpiringCert != nil {
		resp.NotAfter = earliestExpiringCert.NotAfter
	}

	ctx.SetStatusOK()
	return resp, nil
}
