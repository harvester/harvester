package jwe

import (
	"fmt"
	"strconv"
	"time"

	dashboardapi "github.com/kubernetes/dashboard/src/app/backend/auth/api"
	dashboardjwt "github.com/kubernetes/dashboard/src/app/backend/auth/jwe"
	ctlcorev1 "github.com/rancher/wrangler-api/pkg/generated/controllers/core/v1"

	authsync "github.com/rancher/harvester/pkg/auth/sync"
	"github.com/rancher/harvester/pkg/settings"
)

func NewJWETokenManager(secrets ctlcorev1.SecretClient, namespace string) (tokenManager dashboardapi.TokenManager, err error) {
	//handle panic from kubernetes dashboard
	defer func() {
		if recoveryMessage := recover(); recoveryMessage != nil {
			err = fmt.Errorf("%v", recoveryMessage)
		}
	}()

	synchronizer := authsync.NewSecretSynchronizer(secrets, namespace, settings.AuthSecretName.Get())
	keyHolder := dashboardjwt.NewRSAKeyHolder(synchronizer)
	tokenManager = dashboardjwt.NewJWETokenManager(keyHolder)
	tokenManager.SetTokenTTL(GetTokenMaxTTL())
	return
}

func GetTokenMaxTTL() time.Duration {
	ttlStr := settings.AuthTokenMaxTTLMinutes.Get()
	ttl, err := strconv.ParseInt(ttlStr, 10, 32)
	if err != nil {
		ttl = 720
	}
	return time.Duration(ttl) * time.Minute
}
