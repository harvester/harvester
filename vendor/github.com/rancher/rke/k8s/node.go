package k8s

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
)

const (
	HostnameLabel                = "kubernetes.io/hostname"
	InternalAddressAnnotation    = "rke.cattle.io/internal-ip"
	ExternalAddressAnnotation    = "rke.cattle.io/external-ip"
	AWSCloudProvider             = "aws"
	ExternalAWSCloudProviderName = "external-aws"
	MaxRetries                   = 5
	RetryInterval                = 5
)

func DeleteNode(k8sClient *kubernetes.Clientset, nodeName string, nodeAddress string, cloudProviderName string) error {
	// If cloud provider is configured, the node name can be set by the cloud provider, which can be different from the original node name
	if cloudProviderName != "" {
		node, err := GetNode(k8sClient, nodeName, nodeAddress, cloudProviderName)
		if err != nil {
			return err
		}
		nodeName = node.Name
	}
	return k8sClient.CoreV1().Nodes().Delete(context.TODO(), nodeName, metav1.DeleteOptions{})
}

func GetNodeList(k8sClient *kubernetes.Clientset) (*v1.NodeList, error) {
	return k8sClient.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
}

func GetNode(k8sClient *kubernetes.Clientset, nodeName, nodeAddress, cloudProviderName string) (*v1.Node, error) {
	var listErr error
	for retries := 0; retries < MaxRetries; retries++ {
		logrus.Debugf("Checking node list for node [%v], try #%v", nodeName, retries+1)
		nodes, err := GetNodeList(k8sClient)
		if err != nil {
			listErr = err
			time.Sleep(time.Second * RetryInterval)
			continue
		}
		// reset listErr back to nil
		listErr = nil
		for _, node := range nodes.Items {
			if strings.ToLower(node.Labels[HostnameLabel]) == strings.ToLower(nodeName) {
				return &node, nil
			}
			if cloudProviderName == ExternalAWSCloudProviderName {
				if nodeAddress == "" {
					return nil, fmt.Errorf("failed to find node [%v] with empty nodeAddress, cloud provider: %v", nodeName, cloudProviderName)
				}
				logrus.Debugf("Checking internal address for node [%v], cloud provider: %v", nodeName, cloudProviderName)
				for _, addr := range node.Status.Addresses {
					if addr.Type == v1.NodeInternalIP && nodeAddress == addr.Address {
						logrus.Debugf("Found node [%s]: %v", nodeName, nodeAddress)
						return &node, nil
					}
				}
			}
		}
		time.Sleep(time.Second * RetryInterval)
	}
	if listErr != nil {
		return nil, listErr
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{}, nodeName)
}

func CordonUncordon(k8sClient *kubernetes.Clientset, nodeName string, nodeAddress string, cloudProviderName string, cordoned bool) error {
	updated := false
	for retries := 0; retries < MaxRetries; retries++ {
		node, err := GetNode(k8sClient, nodeName, nodeAddress, cloudProviderName)
		if err != nil {
			logrus.Debugf("Error getting node %s: %v", nodeName, err)
			// no need to retry here since GetNode already retries
			return err
		}
		if node.Spec.Unschedulable == cordoned {
			logrus.Debugf("Node %s is already cordoned: %v", nodeName, cordoned)
			return nil
		}
		node.Spec.Unschedulable = cordoned
		_, err = k8sClient.CoreV1().Nodes().Update(context.TODO(), node, metav1.UpdateOptions{})
		if err != nil {
			logrus.Debugf("Error setting cordoned state for node %s: %v", nodeName, err)
			time.Sleep(time.Second * RetryInterval)
			continue
		}
		updated = true
	}
	if !updated {
		return fmt.Errorf("Failed to set cordonded state for node: %s", nodeName)
	}
	return nil
}

func IsNodeReady(node v1.Node) bool {
	nodeConditions := node.Status.Conditions
	for _, condition := range nodeConditions {
		if condition.Type == v1.NodeReady && condition.Status == v1.ConditionTrue {
			return true
		}
	}
	return false
}

func SyncNodeLabels(node *v1.Node, toAddLabels, toDelLabels map[string]string) {
	oldLabels := map[string]string{}
	if node.Labels == nil {
		node.Labels = map[string]string{}
	}

	for k, v := range node.Labels {
		oldLabels[k] = v
	}

	// Delete Labels
	for key := range toDelLabels {
		if _, ok := node.Labels[key]; ok {
			delete(node.Labels, key)
		}
	}

	// ADD Labels
	for key, value := range toAddLabels {
		node.Labels[key] = value
	}
}

func SyncNodeTaints(node *v1.Node, toAddTaints, toDelTaints []string) {
	// Add taints to node
	for _, taintStr := range toAddTaints {
		if isTaintExist(toTaint(taintStr), node.Spec.Taints) {
			continue
		}
		node.Spec.Taints = append(node.Spec.Taints, toTaint(taintStr))
	}
	// Remove Taints from node
	for _, taintStr := range toDelTaints {
		node.Spec.Taints = delTaintFromList(node.Spec.Taints, toTaint(taintStr))
	}
}

func isTaintExist(taint v1.Taint, taintList []v1.Taint) bool {
	for _, t := range taintList {
		if t.Key == taint.Key && t.Value == taint.Value && t.Effect == taint.Effect {
			return true
		}
	}
	return false
}

func toTaint(taintStr string) v1.Taint {
	taintStruct := strings.Split(taintStr, "=")
	tmp := strings.Split(taintStruct[1], ":")
	key := taintStruct[0]
	value := tmp[0]
	effect := v1.TaintEffect(tmp[1])
	return v1.Taint{
		Key:    key,
		Value:  value,
		Effect: effect,
	}
}

func SetNodeAddressesAnnotations(node *v1.Node, internalAddress, externalAddress string) {
	currentExternalAnnotation := node.Annotations[ExternalAddressAnnotation]
	currentInternalAnnotation := node.Annotations[ExternalAddressAnnotation]
	if currentExternalAnnotation == externalAddress && currentInternalAnnotation == internalAddress {
		return
	}
	node.Annotations[ExternalAddressAnnotation] = externalAddress
	node.Annotations[InternalAddressAnnotation] = internalAddress
}

func delTaintFromList(l []v1.Taint, t v1.Taint) []v1.Taint {
	r := []v1.Taint{}
	for _, i := range l {
		if i == t {
			continue
		}
		r = append(r, i)
	}
	return r
}
