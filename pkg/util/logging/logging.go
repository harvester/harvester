package logging

import (
	"fmt"

	loggingv1 "github.com/kube-logging/logging-operator/pkg/sdk/logging/api/v1beta1"
	"k8s.io/apimachinery/pkg/labels"

	ctlloggingv1 "github.com/harvester/harvester/pkg/generated/controllers/logging.banzaicloud.io/v1beta1"
)

type Logging struct {
	flowCache          ctlloggingv1.FlowCache
	outputCache        ctlloggingv1.OutputCache
	clusterFlowCache   ctlloggingv1.ClusterFlowCache
	clusterOutputCache ctlloggingv1.ClusterOutputCache
}

func NewLogging(
	flowCache ctlloggingv1.FlowCache,
	outputCache ctlloggingv1.OutputCache,
	clusterFlowCache ctlloggingv1.ClusterFlowCache,
	clusterOutputCache ctlloggingv1.ClusterOutputCache) *Logging {
	return &Logging{
		flowCache:          flowCache,
		outputCache:        outputCache,
		clusterFlowCache:   clusterFlowCache,
		clusterOutputCache: clusterOutputCache,
	}
}

func (l *Logging) FlowsDangling(ns string) error {
	flowlist, err := l.flowCache.List(ns, labels.NewSelector())
	if err != nil {
		return err
	}
	if len(flowlist) == 0 {
		return nil
	}
	outputlist, err := l.outputCache.List(ns, labels.NewSelector())
	if err != nil {
		return err
	}
	outputs := getOutputs(outputlist)
	for _, flow := range flowlist {
		if err := flowDangling(flow, outputs); err != nil {
			return err
		}
	}

	return nil
}

func (l *Logging) ClusterFlowsDangling(ns string) error {
	flowlist, err := l.clusterFlowCache.List(ns, labels.NewSelector())
	if err != nil {
		return err
	}
	if len(flowlist) == 0 {
		return nil
	}
	outputlist, err := l.clusterOutputCache.List(ns, labels.NewSelector())
	if err != nil {
		return err
	}
	outputs := getClusterOutputs(outputlist)
	for _, flow := range flowlist {
		if err := clusterFlowDangling(flow, outputs); err != nil {
			return err
		}
	}

	return nil
}

func flowDangling(flow *loggingv1.Flow, outputs map[string]string) error {
	for _, gor := range flow.Spec.GlobalOutputRefs {
		lr, ok := outputs[gor]
		if !ok {
			return fmt.Errorf("flow %s/%s has a GlobalOutputRefs %s, the Output does not exist", flow.Namespace, flow.Name, gor)
		}
		// by default, the LoggingRef on normal log is empty, on audit log is a predefined value
		if lr != flow.Spec.LoggingRef {
			return fmt.Errorf("flow %s/%s has a GlobalOutputRefs %s, the LoggingRef is different %s %s", flow.Namespace, flow.Name, gor, flow.Spec.LoggingRef, lr)
		}
	}

	for _, lor := range flow.Spec.LocalOutputRefs {
		lr, ok := outputs[lor]
		if !ok {
			return fmt.Errorf("flow %s/%s has a LocalOutputRefs %s, the output does not exist", flow.Namespace, flow.Name, lor)
		}
		if lr != flow.Spec.LoggingRef {
			return fmt.Errorf("flow %s/%s has a LocalOutputRefs %s, the LoggingRef is different %s %s", flow.Namespace, flow.Name, lor, flow.Spec.LoggingRef, lr)
		}
	}
	return nil
}

func clusterFlowDangling(flow *loggingv1.ClusterFlow, outputs map[string]string) error {
	for _, gor := range flow.Spec.GlobalOutputRefs {
		lr, ok := outputs[gor]
		if !ok {
			return fmt.Errorf("clusterFlow %s/%s has a GlobalOutputRefs %s, the ClusterOutput does not exist", flow.Namespace, flow.Name, gor)
		}
		if lr != flow.Spec.LoggingRef {
			return fmt.Errorf("clusterFlow %s/%s has a GlobalOutputRefs %s, the LoggingRef is different %s %s", flow.Namespace, flow.Name, gor, flow.Spec.LoggingRef, lr)
		}
	}
	return nil
}

func getOutputs(outputList []*loggingv1.Output) map[string]string {
	if len(outputList) == 0 {
		return nil
	}
	outputs := make(map[string]string, len(outputList))
	for _, output := range outputList {
		outputs[output.Name] = output.Spec.LoggingRef
	}
	return outputs
}

func getClusterOutputs(clusterOutputList []*loggingv1.ClusterOutput) map[string]string {
	if len(clusterOutputList) == 0 {
		return nil
	}
	outputs := make(map[string]string, len(clusterOutputList))
	for _, output := range clusterOutputList {
		outputs[output.Name] = output.Spec.LoggingRef
	}
	return outputs
}
