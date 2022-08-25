package upgrade

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/mod/semver"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	lhclientset "github.com/longhorn/longhorn-manager/k8s/pkg/client/clientset/versioned"
	"github.com/longhorn/longhorn-manager/types"
	upgradeutil "github.com/longhorn/longhorn-manager/upgrade/util"

	"github.com/longhorn/longhorn-manager/meta"
	"github.com/longhorn/longhorn-manager/upgrade/v070to080"
	"github.com/longhorn/longhorn-manager/upgrade/v100to101"
	"github.com/longhorn/longhorn-manager/upgrade/v102to110"
	"github.com/longhorn/longhorn-manager/upgrade/v110to111"
	"github.com/longhorn/longhorn-manager/upgrade/v110to120"
	"github.com/longhorn/longhorn-manager/upgrade/v111to120"
	"github.com/longhorn/longhorn-manager/upgrade/v120to121"
	"github.com/longhorn/longhorn-manager/upgrade/v122to123"
	"github.com/longhorn/longhorn-manager/upgrade/v12xto130"
	"github.com/longhorn/longhorn-manager/upgrade/v1beta1"
)

const (
	LeaseLockName = "longhorn-manager-upgrade-lock"
)

func Upgrade(kubeconfigPath, currentNodeID string) error {
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

	if err := upgradeLocalNode(); err != nil {
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

func upgradeLocalNode() (err error) {
	defer func() {
		err = errors.Wrap(err, "upgrade local node failed")
	}()
	if err := v070to080.UpgradeLocalNode(); err != nil {
		return err
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

	if semver.Compare(lhVersionBeforeUpgrade, "v0.8.0") < 0 {
		logrus.Debugf("Walking through the upgrade path v0.7.0 to v0.8.0")
		if err := v070to080.UpgradeResources(namespace, lhClient); err != nil {
			return err
		}
	}
	if semver.Compare(lhVersionBeforeUpgrade, "v1.0.1") < 0 {
		logrus.Debugf("Walking through the upgrade path v1.0.0 to v1.0.1")
		if err := v100to101.UpgradeResources(namespace, lhClient, kubeClient); err != nil {
			return err
		}
	}
	if semver.Compare(lhVersionBeforeUpgrade, "v1.1.0") < 0 {
		logrus.Debugf("Walking through the upgrade path v1.0.2 to v1.1.0")
		if err := v102to110.UpgradeResources(namespace, lhClient, kubeClient); err != nil {
			return err
		}
	}
	if semver.Compare(lhVersionBeforeUpgrade, "v1.1.1") < 0 {
		logrus.Debugf("Walking through the upgrade path v1.1.0 to v1.1.1")
		if err := v110to111.UpgradeResources(namespace, lhClient, kubeClient); err != nil {
			return err
		}
	}
	if semver.Compare(lhVersionBeforeUpgrade, "v1.2.0") < 0 {
		logrus.Debugf("Walking through the upgrade path v1.1.0 to v1.2.0")
		if err := v110to120.UpgradeResources(namespace, lhClient, kubeClient); err != nil {
			return err
		}
	}
	if semver.Compare(lhVersionBeforeUpgrade, "v1.2.0") < 0 {
		logrus.Debugf("Walking through the upgrade path v1.1.1 to v1.2.0")
		if err := v111to120.UpgradeResources(namespace, lhClient); err != nil {
			return err
		}
	}
	if semver.Compare(lhVersionBeforeUpgrade, "v1.2.1") < 0 {
		logrus.Debugf("Walking through the upgrade path v1.2.0 to v1.2.1")
		if err := v120to121.UpgradeResources(namespace, lhClient); err != nil {
			return err
		}
	}
	if semver.Compare(lhVersionBeforeUpgrade, "v1.2.3") < 0 {
		logrus.Debugf("Walking through the upgrade path v1.2.2 to v1.2.3")
		if err := v122to123.UpgradeResources(namespace, lhClient); err != nil {
			return err
		}
	}
	if semver.Compare(lhVersionBeforeUpgrade, "v1.3.0") < 0 {
		logrus.Debugf("Walking through the upgrade path v1.2.x to v1.3.0")
		if err := v12xto130.UpgradeResources(namespace, lhClient, kubeClient); err != nil {
			return err
		}
	}

	return upgradeutil.CreateOrUpdateLonghornVersionSetting(namespace, lhClient)
}
