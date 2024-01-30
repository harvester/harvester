package upgrade

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/mod/semver"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"

	"github.com/longhorn/longhorn-manager/meta"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/upgrade/v14xto150"
	"github.com/longhorn/longhorn-manager/upgrade/v151to152"
	"github.com/longhorn/longhorn-manager/upgrade/v1beta1"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	lhclientset "github.com/longhorn/longhorn-manager/k8s/pkg/client/clientset/versioned"
	upgradeutil "github.com/longhorn/longhorn-manager/upgrade/util"
)

const (
	LeaseLockName = "longhorn-manager-upgrade-lock"
)

func Upgrade(kubeconfigPath, currentNodeID, managerImage string) error {
	namespace := os.Getenv(types.EnvPodNamespace)
	if namespace == "" {
		logrus.Warnf("Cannot detect pod namespace, environment variable %v is missing, "+
			"using default namespace", types.EnvPodNamespace)
		namespace = corev1.NamespaceDefault
	}
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return errors.Wrap(err, "unable to get client config")
	}

	// There is only one leading Longhorn manager that is doing modification to the CRs.
	// Increase this value so that leading Longhorn manager can finish upgrading faster
	config.Burst = 1000
	config.QPS = 1000

	kubeClient, err := clientset.NewForConfig(config)
	if err != nil {
		return errors.Wrap(err, "unable to get k8s client")
	}

	lhClient, err := lhclientset.NewForConfig(config)
	if err != nil {
		return errors.Wrap(err, "unable to get clientset")
	}

	scheme := runtime.NewScheme()
	if err := longhorn.SchemeBuilder.AddToScheme(scheme); err != nil {
		return errors.Wrap(err, "unable to create scheme")
	}

	if err := upgradeutil.CheckUpgradePathSupported(namespace, lhClient); err != nil {
		return err
	}

	if err := waitForOldLonghornManagersToBeFullyRemoved(namespace, managerImage, kubeClient); err != nil {
		return err
	}

	if err := upgrade(currentNodeID, namespace, config, lhClient, kubeClient); err != nil {
		return err
	}

	return nil
}

func upgrade(currentNodeID, namespace string, config *restclient.Config, lhClient *lhclientset.Clientset, kubeClient *clientset.Clientset) error {
	ctx, cancel := context.WithCancel(context.Background())
	var err error
	defer cancel()

	// If the current Longhorn is already the latest version,
	// the leader election & the whole upgrade path could be skipped.
	lhVersionBeforeUpgrade, err := upgradeutil.GetCurrentLonghornVersion(namespace, lhClient)
	if err != nil {
		return err
	}
	if semver.IsValid(meta.Version) && semver.Compare(lhVersionBeforeUpgrade, meta.Version) >= 0 {
		logrus.Infof("Skip the leader election for the upgrade since the current Longhorn system is already up to date")
		return nil
	}

	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      LeaseLockName,
			Namespace: namespace,
		},
		Client: kubeClient.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: currentNodeID,
		},
	}

	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock:            lock,
		ReleaseOnCancel: true,
		LeaseDuration:   20 * time.Second,
		RenewDeadline:   10 * time.Second,
		RetryPeriod:     2 * time.Second,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				defer cancel()
				defer func() {
					if err != nil {
						logrus.Errorf("Upgrade failed: %v", err)
					} else {
						logrus.Infof("Finish upgrading")
					}
				}()
				logrus.Infof("Start upgrading")
				if err = doAPIVersionUpgrade(namespace, config, lhClient); err != nil {
					return
				}
				if err = doResourceUpgrade(namespace, lhClient, kubeClient); err != nil {
					return
				}
			},
			OnStoppedLeading: func() {
				logrus.Infof("Upgrade leader lost: %s", currentNodeID)
			},
			OnNewLeader: func(identity string) {
				if identity == currentNodeID {
					return
				}
				logrus.Infof("New upgrade leader elected: %s", identity)
			},
		},
	})

	return err
}

func doAPIVersionUpgrade(namespace string, config *restclient.Config, lhClient *lhclientset.Clientset) (err error) {
	defer func() {
		err = errors.Wrap(err, "upgrade API version failed")
	}()

	crdAPIVersion := ""

	crdAPIVersionSetting, err := lhClient.LonghornV1beta2().Settings(namespace).Get(context.TODO(), string(types.SettingNameCRDAPIVersion), metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
	} else {
		crdAPIVersion = crdAPIVersionSetting.Value
	}

	if crdAPIVersion != "" &&
		crdAPIVersion != types.CRDAPIVersionV1beta1 &&
		crdAPIVersion != types.CRDAPIVersionV1beta2 {
		return fmt.Errorf("unrecognized CRD API version %v", crdAPIVersion)
	}

	if crdAPIVersion == types.CurrentCRDAPIVersion {
		logrus.Info("No API version upgrade is needed")
		return nil
	}

	switch crdAPIVersion {
	case "":
		// upgradable: new installation
		// non-upgradable: error or non-supported version (v1alpha1 which cannot upgrade directly)
		upgradable, err := v1beta1.CanUpgrade(config, namespace)
		if err != nil {
			return err
		}

		if upgradable {
			crdAPIVersionSetting = &longhorn.Setting{
				ObjectMeta: metav1.ObjectMeta{
					Name: string(types.SettingNameCRDAPIVersion),
				},
				Value: types.CurrentCRDAPIVersion,
			}
			_, err = lhClient.LonghornV1beta2().Settings(namespace).Create(context.TODO(), crdAPIVersionSetting, metav1.CreateOptions{})
			if err != nil && !apierrors.IsAlreadyExists(err) {
				return errors.Wrap(err, "cannot create CRDAPIVersionSetting")
			}
			logrus.Infof("New %v installation", types.CurrentCRDAPIVersion)
		}
	case types.CRDAPIVersionV1beta1:
		logrus.Infof("Upgrading from %v to %v", types.CRDAPIVersionV1beta1, types.CurrentCRDAPIVersion)
		if err := v1beta1.FixupCRs(config, namespace, lhClient); err != nil {
			return err
		}
		crdAPIVersionSetting.Value = types.CRDAPIVersionV1beta2
		if _, err := lhClient.LonghornV1beta2().Settings(namespace).Update(context.TODO(), crdAPIVersionSetting, metav1.UpdateOptions{}); err != nil {
			return errors.Wrapf(err, "cannot finish CRD API upgrade by setting the CRDAPIVersionSetting to %v", types.CurrentCRDAPIVersion)
		}
		logrus.Infof("CRD has been upgraded to %v", crdAPIVersionSetting.Value)
	default:
		return fmt.Errorf("don't support upgrade from %v to %v", crdAPIVersion, types.CurrentCRDAPIVersion)
	}

	return nil
}

func doResourceUpgrade(namespace string, lhClient *lhclientset.Clientset, kubeClient *clientset.Clientset) (err error) {
	defer func() {
		err = errors.Wrap(err, "upgrade resources failed")
	}()

	lhVersionBeforeUpgrade, err := upgradeutil.GetCurrentLonghornVersion(namespace, lhClient)
	if err != nil {
		return err
	}

	// When lhVersionBeforeUpgrade < v1.5.0, it is v1.4.x. The `CheckUpgradePathSupported` method would have failed us out earlier if it was not v1.4.x.
	resourceMaps := map[string]interface{}{}
	if semver.Compare(lhVersionBeforeUpgrade, "v1.5.0") < 0 {
		logrus.Info("Walking through the resource upgrade path v1.4.x to v1.5.0")
		if err := v14xto150.UpgradeResources(namespace, lhClient, kubeClient, resourceMaps); err != nil {
			return err
		}
	}
	if semver.Compare(lhVersionBeforeUpgrade, "v1.5.2") < 0 {
		logrus.Info("Walking through the resource upgrade path v1.5.1 to v1.5.2")
		if err := v151to152.UpgradeResources(namespace, lhClient, kubeClient, resourceMaps); err != nil {
			return err
		}
	}
	if err := upgradeutil.UpdateResources(namespace, lhClient, resourceMaps); err != nil {
		return err
	}

	// When lhVersionBeforeUpgrade < v1.5.0, it is v1.4.x. The `CheckUpgradePathSupported` method would have failed us out earlier if it was not v1.4.x.
	resourceMaps = map[string]interface{}{}
	if semver.Compare(lhVersionBeforeUpgrade, "v1.5.0") < 0 {
		logrus.Info("Walking through the resource status upgrade path v1.4.x to v1.5.0")
		if err := v14xto150.UpgradeResourcesStatus(namespace, lhClient, kubeClient, resourceMaps); err != nil {
			return err
		}
	}
	if err := upgradeutil.UpdateResourcesStatus(namespace, lhClient, resourceMaps); err != nil {
		return err
	}

	if err := upgradeutil.DeleteRemovedSettings(namespace, lhClient); err != nil {
		return err
	}

	return upgradeutil.CreateOrUpdateLonghornVersionSetting(namespace, lhClient)
}

func waitForOldLonghornManagersToBeFullyRemoved(namespace, managerImage string, kubeClient *clientset.Clientset) error {
	logrus.Info("Waiting for old Longhorn manager pods to be fully removed")
	for i := 0; i < 600; i++ {
		managerPods, err := upgradeutil.ListManagerPods(namespace, kubeClient)
		if err != nil {
			return err
		}
		foundOldManager := false
		for _, pod := range managerPods {
			if isOldPod, oldImage := isOldManagerPod(pod, managerImage); isOldPod {
				logrus.Infof("Found old longhorn manager: %v with image %v", pod.Name, oldImage)
				foundOldManager = true
				break
			}
		}
		if !foundOldManager {
			return nil
		}
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("timed out while waiting for old Longhorn manager pods to be fully removed")
}

func isOldManagerPod(pod corev1.Pod, managerImage string) (bool, string) {
	for _, container := range pod.Spec.Containers {
		if container.Name == "longhorn-manager" {
			if container.Image != managerImage {
				return true, container.Image
			}
		}
	}
	return false, ""
}
