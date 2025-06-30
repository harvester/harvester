package restorevm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"

	kubevirtv1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"

	"github.com/harvester/harvester/pkg/generated/clientset/versioned/scheme"
	ctlharvester "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

const (
	RestoreVMCompleted = "RestoreVMCompleted"
	RestoreVMFailed    = "RestoreVMFailed"
)

var healthzPath = "/apis/" + kubevirtv1.SubresourceGroupName + "/" + kubevirtv1.ApiLatestVersion + "/healthz"

type RestoreVMHandler struct {
	kubeConfig  string
	kubeContext string

	nodeName    string
	upgradeName string

	virtClient   kubecli.KubevirtClient
	factory      *ctlharvester.Factory
	upgradeCache ctlharvesterv1.UpgradeCache

	vmRestClient *rest.RESTClient
	k8sClient    *kubernetes.Clientset
	recorder     record.EventRecorder
}

func NewRestoreVMHandler(kubeConfig, kubeContext, nodeName, upgrade string) (*RestoreVMHandler, error) {
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		logrus.Fatalf("failed to build REST config: %v", err)
	}

	virtClient, err := kubecli.GetKubevirtClientFromRESTConfig(restConfig)
	if err != nil {
		logrus.Fatalf("failed to get kubevirt client: %v", err)
	}

	k8sClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		logrus.Fatalf("failed to create Kubernetes client: %v", err)
	}

	factory, err := ctlharvester.NewFactoryFromConfig(restConfig)
	if err != nil {
		logrus.Fatalf("cannot obtain harvester factory: %v", err)
	}

	broadcaster := record.NewBroadcaster()
	broadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: virtClient.CoreV1().Events(util.HarvesterSystemNamespaceName)})
	recorder := broadcaster.NewRecorder(
		scheme.Scheme,
		corev1.EventSource{Component: "restore-vm", Host: nodeName},
	)

	return &RestoreVMHandler{
		kubeConfig:   kubeConfig,
		kubeContext:  kubeContext,
		nodeName:     nodeName,
		upgradeName:  upgrade,
		virtClient:   virtClient,
		factory:      factory,
		upgradeCache: factory.Harvesterhci().V1beta1().Upgrade().Cache(),
		vmRestClient: virtClient.RestClient(),
		k8sClient:    k8sClient,
		recorder:     recorder,
	}, nil
}

func (h *RestoreVMHandler) Run(ctx context.Context) error {
	defer func() {
		// wait for events to be flushed
		time.Sleep(10 * time.Second)
	}()

	if err := h.factory.Sync(ctx); err != nil {
		return fmt.Errorf("failed to sync factory: %w", err)
	}

	vmNames, err := h.getVMNamesFromConfigMap(ctx)
	if err != nil {
		if errors.IsNotFound(err) {
			logrus.Warn("ConfigMap not found")
			h.recordUpgradeEvent(corev1.EventTypeWarning, RestoreVMFailed, "ConfigMap not found")
			return nil // ConfigMap not found, nothing to restore
		}
		return err
	}
	if len(vmNames) == 0 {
		logrus.Info("No VMs to restore")
		h.recordUpgradeEvent(corev1.EventTypeNormal, RestoreVMCompleted,
			"Restored 0 VM for node %s during upgrade %s", h.nodeName, h.upgradeName)
		return nil
	}

	if err := h.checkKubeVirtHealth(ctx); err != nil {
		return fmt.Errorf("KubeVirt not ready: %w", err)
	}

	vmSuccessCnt := 0
	vmFailedCnt := 0
	for _, vmFullName := range vmNames {
		parts := strings.SplitN(vmFullName, "/", 2)
		if len(parts) != 2 {
			logrus.Errorf("Invalid VM name: %s, should be namespace/name", vmFullName)
			continue
		}
		ns, name := parts[0], parts[1]
		logrus.Infof("Starting VM %s/%s...", ns, name)
		if err := h.startVM(ctx, ns, name); err != nil {
			logrus.Errorf("Failed to start VM %s/%s: %v", ns, name, err)
			h.recordUpgradeEvent(corev1.EventTypeWarning, RestoreVMFailed,
				"Failed to restore VM %s/%s for node %s during upgrade %s: %v", ns, name, h.nodeName, h.upgradeName, err)
			vmFailedCnt++
		} else {
			vmSuccessCnt++
		}
	}

	h.recordUpgradeEvent(corev1.EventTypeNormal, RestoreVMCompleted,
		"Restored %d VMs for node %s during upgrade %s, success: %d, failed: %d", len(vmNames), h.nodeName, h.upgradeName, vmSuccessCnt, vmFailedCnt)
	return nil
}

func (h *RestoreVMHandler) checkKubeVirtHealth(ctx context.Context) error {
	logrus.Infof("Waiting for KubeVirt to be ready...")
	return wait.PollUntilContextTimeout(ctx, 5*time.Second, 30*time.Minute, true, func(ctx context.Context) (bool, error) {
		res := h.vmRestClient.Get().AbsPath(healthzPath).Do(ctx)
		if res.Error() != nil {
			logrus.Errorf("KubeVirt health check failed: %v, retry...", res.Error())
			return false, nil // keep retrying
		}
		return true, nil
	})
}

func (h *RestoreVMHandler) getVMNamesFromConfigMap(ctx context.Context) ([]string, error) {
	cmName := util.GetRestoreVMConfigMapName(h.upgradeName)
	cm, err := h.k8sClient.CoreV1().ConfigMaps(util.HarvesterSystemNamespaceName).Get(ctx, cmName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get configmap: %w", err)
	}
	vmNamesStr, ok := cm.Data[h.nodeName]
	// handle empty string case
	if !ok || strings.TrimSpace(vmNamesStr) == "" {
		return []string{}, nil
	}
	// split and filter out empty names
	rawNames := strings.Split(vmNamesStr, ",")
	vmNames := make([]string, 0, len(rawNames))
	for _, name := range rawNames {
		name = strings.TrimSpace(name)
		if name != "" {
			vmNames = append(vmNames, name)
		}
	}
	return vmNames, nil
}

func (h *RestoreVMHandler) startVM(ctx context.Context, namespace, name string) error {
	return h.virtClient.VirtualMachine(namespace).Start(ctx, name, &kubevirtv1.StartOptions{})
}

func (h *RestoreVMHandler) recordUpgradeEvent(eventType, reason, messageFmt string, args ...interface{}) {
	upgrade, err := h.upgradeCache.Get(util.HarvesterSystemNamespaceName, h.upgradeName)
	if err != nil {
		logrus.Warnf("Record upgrade events failed: %v", err)
		return
	}
	logrus.Info("Recording event for upgrade", h.upgradeName, ":", eventType, reason, fmt.Sprintf(messageFmt, args...))
	h.recorder.Eventf(upgrade, eventType, reason, messageFmt, args...)
}
