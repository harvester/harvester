package aggregation

import (
	"bytes"
	"context"
	"net/http"

	v1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

func Watch(ctx context.Context, controller v1.SecretController, secretNamespace, secretName string, httpHandler http.Handler) {
	if secretNamespace == "" || secretName == "" {
		return
	}
	h := handler{
		ctx:       ctx,
		handler:   httpHandler,
		namespace: secretNamespace,
		name:      secretName,
	}
	controller.OnChange(ctx, "aggregation-controller", h.OnSecret)
}

type handler struct {
	handler         http.Handler
	namespace, name string

	url    string
	caCert []byte
	token  string
	ctx    context.Context
	cancel func()
}

func (h *handler) OnSecret(key string, secret *corev1.Secret) (*corev1.Secret, error) {
	if secret == nil {
		return nil, nil
	}

	if secret.Namespace != h.namespace ||
		secret.Name != h.name {
		return secret, nil
	}

	url, caCert, token, restart, err := h.shouldRestart(secret)
	if err != nil {
		return secret, err
	}
	if !restart {
		return secret, nil
	}

	if h.cancel != nil {
		logrus.Info("Restarting steve aggregation client")
		h.cancel()
	} else {
		logrus.Info("Starting steve aggregation client")
	}

	ctx, cancel := context.WithCancel(h.ctx)
	go ListenAndServe(ctx, url, caCert, token, h.handler)

	h.url = url
	h.caCert = caCert
	h.token = token
	h.cancel = cancel

	return secret, nil
}

func (h *handler) shouldRestart(secret *corev1.Secret) (string, []byte, string, bool, error) {
	url := string(secret.Data["url"])
	if url == "" {
		return "", nil, "", false, nil
	}

	token := string(secret.Data["token"])
	if token == "" {
		return "", nil, "", false, nil
	}

	caCert := secret.Data["ca.crt"]

	if h.url != url ||
		h.token != token ||
		!bytes.Equal(h.caCert, caCert) {
		return url, caCert, token, true, nil
	}

	return "", nil, "", false, nil
}
