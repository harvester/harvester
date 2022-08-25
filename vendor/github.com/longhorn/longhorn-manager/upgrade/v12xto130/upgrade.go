package v12xto130

import (
	"context"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"

	lhclientset "github.com/longhorn/longhorn-manager/k8s/pkg/client/clientset/versioned"
)

const (
	upgradeLogPrefix = "upgrade from v1.2.x to v1.3.0: "
)

func UpgradeResources(namespace string, lhClient *lhclientset.Clientset, kubeClient *clientset.Clientset) error {
	if err := upgradeReplicas(namespace, lhClient); err != nil {
		return err
	}
	return nil
}

func upgradeReplicas(namespace string, lhClient *lhclientset.Clientset) (err error) {
	defer func() {
		err = errors.Wrapf(err, upgradeLogPrefix+"upgrade replica failed")
	}()

	replicas, err := lhClient.LonghornV1beta2().Replicas(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to list all existing Longhorn replicas during the replica upgrade")
	}

	for _, r := range replicas.Items {
		if r.Status.IP == "" {
			continue
		}

		if r.Status.StorageIP != "" {
			continue
		}

		r.Status.StorageIP = r.Status.IP
		_, err = lhClient.LonghornV1beta2().Replicas(namespace).Update(context.TODO(), &r, metav1.UpdateOptions{})
		if err != nil {
			return errors.Wrapf(err, "failed to update %v storage IP", r.Name)
		}

		logrus.Debugf(upgradeLogPrefix+"Updated %v storage IP to %v", r.Name, r.Status.StorageIP)
	}

	return nil
}
