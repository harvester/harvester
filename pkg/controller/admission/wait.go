package admission

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"

	"github.com/harvester/harvester/pkg/webhook"
)

const (
	PollingInterval = 5 * time.Second
	PollingTimeout  = 3 * time.Minute
)

// Wait waits for the admission webhook server to register ValidatingWebhookConfiguration and MutatingWebhookConfiguration resources.
func Wait(ctx context.Context, clientSet *kubernetes.Clientset) error {
	return wait.PollImmediate(PollingInterval, PollingTimeout, func() (bool, error) {
		logrus.Infof("Waiting for ValidatingWebhookConfiguration %s...", webhook.ValidatingWebhookName)
		_, err := clientSet.AdmissionregistrationV1().ValidatingWebhookConfigurations().Get(ctx, webhook.ValidatingWebhookName, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}

		logrus.Infof("Waiting for MutatingWebhookConfiguration %s...", webhook.MutatingWebhookName)
		_, err = clientSet.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(ctx, webhook.MutatingWebhookName, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}

		logrus.Infof("Admission webhooks are ready")
		return true, nil
	})
}
