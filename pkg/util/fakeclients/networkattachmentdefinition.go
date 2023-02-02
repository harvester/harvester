package fakeclients

import (
	"context"

	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	cnitype "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/k8s.cni.cncf.io/v1"
	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
)

type NetworkAttachmentDefinitionCache func(namespace string) cnitype.NetworkAttachmentDefinitionInterface

func (c NetworkAttachmentDefinitionCache) Get(namespace, name string) (*cniv1.NetworkAttachmentDefinition, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c NetworkAttachmentDefinitionCache) List(namespace string, selector labels.Selector) ([]*cniv1.NetworkAttachmentDefinition, error) {
	nadList, err := c(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}

	nads := []*cniv1.NetworkAttachmentDefinition{}
	for _, item := range nadList.Items {
		nads = append(nads, &item)
	}

	return nads, nil
}

func (c NetworkAttachmentDefinitionCache) AddIndexer(indexName string, indexer ctlcniv1.NetworkAttachmentDefinitionIndexer) {
	panic("implement me")
}

func (c NetworkAttachmentDefinitionCache) GetByIndex(indexName, key string) ([]*cniv1.NetworkAttachmentDefinition, error) {
	panic("implement me")
}
