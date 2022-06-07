package v1beta1

import (
	"context"
	"fmt"
	"reflect"

	"github.com/jinzhu/copier"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	restclient "k8s.io/client-go/rest"

	longhornV1beta1 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
	lhclientset "github.com/longhorn/longhorn-manager/k8s/pkg/client/clientset/versioned"
	"github.com/longhorn/longhorn-manager/types"
)

const (
	fixupCRLogPrefix = "fix up CRs:"
)

// FixupCRs initialize the old CR spec which is with the `null` field.
// Since start from longhorn.io/v1beta2, we introduce the CRD structural schema.
// However, for the Kubernetes < 1.20, there is an issue that it does not allow
// the `null` field because of the CRD validation failure.
// Reference issue: https://github.com/kubernetes/kubernetes/issues/86811`
func FixupCRs(config *restclient.Config, namespace string, lhClient *lhclientset.Clientset) (err error) {
	defer func() {
		err = errors.Wrapf(err, fixupCRLogPrefix+" failed")
	}()

	if err := fixupVolumes(namespace, lhClient); err != nil {
		return err
	}
	if err := fixupNodes(namespace, lhClient); err != nil {
		return err
	}
	if err := fixupEngines(namespace, lhClient); err != nil {
		return err
	}
	if err := fixupRecurringJobs(namespace, lhClient); err != nil {
		return err
	}
	if err := fixupBackups(namespace, lhClient); err != nil {
		return err
	}
	if err := fixupBackingImageDataSources(namespace, lhClient); err != nil {
		return err
	}
	if err := fixupBackingImageManagers(namespace, lhClient); err != nil {
		return err
	}
	if err := fixupBackingImages(namespace, lhClient); err != nil {
		return err
	}

	logrus.Infof("%v completed", fixupCRLogPrefix)
	return nil
}

func CanUpgrade(config *restclient.Config, namespace string) (bool, error) {
	lhClient, err := lhclientset.NewForConfig(config)
	if err != nil {
		return false, errors.Wrap(err, "unable to get clientset for v1beta1")
	}

	scheme := runtime.NewScheme()
	if err := longhornV1beta1.SchemeBuilder.AddToScheme(scheme); err != nil {
		return false, errors.Wrap(err, "unable to create scheme for v1beta1")
	}

	_, err = lhClient.LonghornV1beta1().Settings(namespace).Get(context.TODO(), string(types.SettingNameDefaultEngineImage), metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			logrus.Infof("setting %v not found", string(types.SettingNameDefaultEngineImage))
			return true, nil
		}

		return false, errors.Wrap(err, fmt.Sprintf("unable to get setting %v", string(types.SettingNameDefaultEngineImage)))
	}

	// The CRD API version is v1alpha1 if SettingNameCRDAPIVersion is "" and SettingNameDefaultEngineImage is set.
	// Longhorn no longer supports the upgrade from v1alpha1 to v1beta2 directly.
	return false, errors.Wrapf(err, "unable to upgrade from v1alpha1 directly")
}

func fixupVolumes(namespace string, lhClient *lhclientset.Clientset) error {
	volumes, err := lhClient.LonghornV1beta1().Volumes(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, obj := range volumes.Items {
		existing := obj.DeepCopy()

		if obj.Spec.ReplicaAutoBalance == "" {
			obj.Spec.ReplicaAutoBalance = longhornV1beta1.ReplicaAutoBalanceIgnored
		}
		if obj.Spec.AccessMode == "" {
			obj.Spec.AccessMode = longhornV1beta1.AccessModeReadWriteOnce
		}
		if obj.Spec.DiskSelector == nil {
			obj.Spec.DiskSelector = []string{}
		}
		if obj.Spec.NodeSelector == nil {
			obj.Spec.NodeSelector = []string{}
		}
		if obj.Spec.RecurringJobs == nil {
			obj.Spec.RecurringJobs = make([]longhornV1beta1.VolumeRecurringJobSpec, 0)
		}
		for i, src := range obj.Spec.RecurringJobs {
			dst := longhornV1beta1.VolumeRecurringJobSpec{}
			if err := copier.Copy(&dst, &src); err != nil {
				return err
			}
			if dst.Groups == nil {
				dst.Groups = []string{}
			}
			if dst.Labels == nil {
				dst.Labels = make(map[string]string, 0)
			}
			obj.Spec.RecurringJobs[i] = dst
		}
		if reflect.DeepEqual(obj, existing) {
			continue
		}
		if _, err = lhClient.LonghornV1beta1().Volumes(namespace).Update(context.TODO(), &obj, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}
	return nil
}

func fixupNodes(namespace string, lhClient *lhclientset.Clientset) error {
	nodes, err := lhClient.LonghornV1beta1().Nodes(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, obj := range nodes.Items {
		existing := obj.DeepCopy()

		if obj.Spec.Disks == nil {
			obj.Spec.Disks = make(map[string]longhornV1beta1.DiskSpec, 0)
		}
		for key, src := range obj.Spec.Disks {
			if src.Tags == nil {
				dst := longhornV1beta1.DiskSpec{}
				if err := copier.Copy(&dst, &src); err != nil {
					return err
				}
				dst.Tags = []string{}
				obj.Spec.Disks[key] = dst
			}
		}
		if obj.Spec.Tags == nil {
			obj.Spec.Tags = []string{}
		}
		if reflect.DeepEqual(obj, existing) {
			continue
		}
		if _, err = lhClient.LonghornV1beta1().Nodes(namespace).Update(context.TODO(), &obj, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}
	return nil
}

func fixupEngines(namespace string, lhClient *lhclientset.Clientset) error {
	engines, err := lhClient.LonghornV1beta1().Engines(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, obj := range engines.Items {
		existing := obj.DeepCopy()

		if obj.Spec.ReplicaAddressMap == nil {
			obj.Spec.ReplicaAddressMap = make(map[string]string, 0)
		}
		if obj.Spec.UpgradedReplicaAddressMap == nil {
			obj.Spec.UpgradedReplicaAddressMap = make(map[string]string, 0)
		}
		if reflect.DeepEqual(obj, existing) {
			continue
		}
		if _, err = lhClient.LonghornV1beta1().Engines(namespace).Update(context.TODO(), &obj, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}
	return nil
}

func fixupRecurringJobs(namespace string, lhClient *lhclientset.Clientset) error {
	recurringJobs, err := lhClient.LonghornV1beta1().RecurringJobs(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, obj := range recurringJobs.Items {
		existing := obj.DeepCopy()

		if obj.Spec.Name == "" {
			obj.Spec.Name = obj.Name
		}
		if obj.Spec.Groups == nil {
			obj.Spec.Groups = []string{}
		}
		if obj.Spec.Labels == nil {
			obj.Spec.Labels = make(map[string]string, 0)
		}
		if reflect.DeepEqual(obj, existing) {
			continue
		}
		if _, err = lhClient.LonghornV1beta1().RecurringJobs(namespace).Update(context.TODO(), &obj, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}
	return nil
}

func fixupBackups(namespace string, lhClient *lhclientset.Clientset) error {
	backups, err := lhClient.LonghornV1beta1().Backups(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, obj := range backups.Items {
		existing := obj.DeepCopy()

		if obj.Spec.Labels == nil {
			obj.Spec.Labels = make(map[string]string, 0)
		}
		if reflect.DeepEqual(obj, existing) {
			continue
		}
		if _, err = lhClient.LonghornV1beta1().Backups(namespace).Update(context.TODO(), &obj, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}
	return nil
}

func fixupBackingImageDataSources(namespace string, lhClient *lhclientset.Clientset) error {
	backingImageDataSources, err := lhClient.LonghornV1beta1().BackingImageDataSources(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, obj := range backingImageDataSources.Items {
		existing := obj.DeepCopy()

		if obj.Spec.SourceType == "" {
			obj.Spec.SourceType = longhornV1beta1.BackingImageDataSourceTypeDownload
		}
		if obj.Spec.Parameters == nil {
			obj.Spec.Parameters = make(map[string]string, 0)
		}
		if reflect.DeepEqual(obj, existing) {
			continue
		}
		if _, err = lhClient.LonghornV1beta1().BackingImageDataSources(namespace).Update(context.TODO(), &obj, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}
	return nil
}

func fixupBackingImageManagers(namespace string, lhClient *lhclientset.Clientset) error {
	backingImageManagers, err := lhClient.LonghornV1beta1().BackingImageManagers(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, obj := range backingImageManagers.Items {
		existing := obj.DeepCopy()

		if obj.Spec.BackingImages == nil {
			obj.Spec.BackingImages = make(map[string]string, 0)
		}
		if reflect.DeepEqual(obj, existing) {
			continue
		}
		if _, err = lhClient.LonghornV1beta1().BackingImageManagers(namespace).Update(context.TODO(), &obj, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}
	return nil
}

func fixupBackingImages(namespace string, lhClient *lhclientset.Clientset) error {
	backingImages, err := lhClient.LonghornV1beta1().BackingImages(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, obj := range backingImages.Items {
		existing := obj.DeepCopy()

		if obj.Spec.SourceType == "" {
			obj.Spec.SourceType = longhornV1beta1.BackingImageDataSourceTypeDownload
		}
		if obj.Spec.Disks == nil {
			obj.Spec.Disks = make(map[string]struct{}, 0)
		}
		if obj.Spec.SourceParameters == nil {
			obj.Spec.SourceParameters = make(map[string]string, 0)
		}
		if reflect.DeepEqual(obj, existing) {
			continue
		}
		if _, err = lhClient.LonghornV1beta1().BackingImages(namespace).Update(context.TODO(), &obj, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}
	return nil
}
