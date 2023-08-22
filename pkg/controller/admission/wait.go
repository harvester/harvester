package admission

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	pod "k8s.io/kubernetes/pkg/api/v1/pod"

	"github.com/harvester/harvester/pkg/util"
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

// wait until at least one harvester-webhook POD is ready, otherwise, Harvester POD will panic when creating data which has webhook
func WaitHarvesterWebhookPod(ctx context.Context, clientSet *kubernetes.Clientset) error {
	// log error instead of return error, try continuously until success or timeout
	err := wait.PollImmediate(PollingInterval, PollingTimeout, func() (bool, error) {
		logrus.Infof("Waiting for harvester-webhook POD ...")
		// get harvester-webhook related pods
		podList, err := clientSet.CoreV1().Pods(util.HarvesterSystemNamespaceName).List(ctx, metav1.ListOptions{
			LabelSelector: labels.Set{
				"app.kubernetes.io/name":      "harvester",
				"app.kubernetes.io/component": "webhook-server",
			}.String(),
		})
		if err != nil {
			logrus.Infof("List harvester-webhook POD failed with %v", err)
			return false, nil
		}
		if podList == nil {
			return false, nil
		}
		for i := range podList.Items {
			if pod.IsPodReady(&podList.Items[i]) {
				logrus.Infof("Harvester-webhook POD %s is ready", podList.Items[i].Name)
				return true, nil
			}
		}

		return false, nil
	})

	if err != nil {
		logrus.Infof("Faild to wait for harvester-webhook POD, error %v", err)
		return err
	}

	return nil
}
