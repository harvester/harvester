package main

import (
	"context"
	"fmt"

	"github.com/rancher/wrangler/pkg/kv"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/component-helpers/scheduling/corev1"
	kubevirtv1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"
)

type vmLiveMigrateDetector struct {
	kubeConfig  string
	kubeContext string

	nodeName string
	shutdown bool

	virtClient kubecli.KubevirtClient
}

func newVMLiveMigrateDetector(options detectorOptions) *vmLiveMigrateDetector {
	return &vmLiveMigrateDetector{
		kubeConfig:  options.kubeConfigPath,
		kubeContext: options.kubeContext,
		nodeName:    options.nodeName,
		shutdown:    options.shutdown,
	}
}

func (d *vmLiveMigrateDetector) init() (err error) {
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{
			ExplicitPath: d.kubeConfig,
		},
		&clientcmd.ConfigOverrides{
			ClusterInfo:    clientcmdapi.Cluster{},
			CurrentContext: d.kubeContext,
		},
	)

	d.virtClient, err = kubecli.GetKubevirtClientFromClientConfig(clientConfig)
	if err != nil {
		logrus.Fatalf("cannot obtain KubeVirt client: %v\n", err)
	}

	return
}

func (d *vmLiveMigrateDetector) getFilteredNodes(nodes []v1.Node) ([]v1.Node, error) {
	var found bool
	var filteredNodes []v1.Node
	for _, node := range nodes {
		if d.nodeName != node.Name {
			filteredNodes = append(filteredNodes, node)
			continue
		}
		found = true
	}
	if !found {
		return filteredNodes, fmt.Errorf("node %s not found", d.nodeName)
	}
	return filteredNodes, nil
}

func (d *vmLiveMigrateDetector) getNonMigratableVMs(ctx context.Context, nodes []v1.Node) ([]string, error) {
	var vmNames []string

	labelSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{
			"kubevirt.io/nodeName": d.nodeName,
		},
	}
	listOptions := metav1.ListOptions{
		LabelSelector: labels.Set(labelSelector.MatchLabels).String(),
	}
	vmis, err := d.virtClient.VirtualMachineInstance("").List(ctx, &listOptions)
	if err != nil {
		return vmNames, err
	}

	for _, vmi := range vmis.Items {
		logrus.Debugf("%s/%s", vmi.Namespace, vmi.Name)

		// Check nodeSelector
		if vmi.Spec.NodeSelector != nil {
			vmNames = append(vmNames, vmi.Namespace+"/"+vmi.Name)
			continue
		}

		// Check pcidevices
		if len(vmi.Spec.Domain.Devices.HostDevices) > 0 {
			vmNames = append(vmNames, vmi.Namespace+"/"+vmi.Name)
			continue
		}

		// Check nodeAffinity
		var matched bool
		if vmi.Spec.Affinity == nil || vmi.Spec.Affinity.NodeAffinity == nil {
			continue
		}
		nodeSelectorTerms := vmi.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
		for i := range nodes {
			matched, err = corev1.MatchNodeSelectorTerms(&nodes[i], nodeSelectorTerms)
			if err != nil {
				return vmNames, fmt.Errorf(err.Error())
			}
			if !matched {
				continue
			}
			if nodes[i].Spec.Unschedulable {
				matched = false
				continue
			}
		}
		if !matched {
			vmNames = append(vmNames, vmi.Namespace+"/"+vmi.Name)
		}
	}
	return vmNames, nil
}

func (d *vmLiveMigrateDetector) run(ctx context.Context) error {
	if d.nodeName == "" {
		return fmt.Errorf("please specify a node name")
	}

	nodeList, err := d.virtClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	filteredNodes, err := d.getFilteredNodes(nodeList.Items)
	if err != nil {
		return err
	}

	logrus.Infof("Checking vms on node %s...", d.nodeName)

	nonLiveMigratableVMNames, err := d.getNonMigratableVMs(ctx, filteredNodes)
	if err != nil {
		return err
	}

	logrus.Infof("Non-migratable VM(s): %v", nonLiveMigratableVMNames)

	if d.shutdown {
		for _, namespacedName := range nonLiveMigratableVMNames {
			namespace, name := kv.RSplit(namespacedName, "/")
			if err := d.virtClient.VirtualMachine(namespace).Stop(ctx, name, &kubevirtv1.StopOptions{}); err != nil {
				return err
			}
			logrus.Infof("vm %s was administratively stopped", namespacedName)
		}
	}

	return nil
}
