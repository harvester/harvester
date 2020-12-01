package node

import (
	"context"
	"testing"

	ctlappsv1 "github.com/rancher/wrangler-api/pkg/generated/controllers/apps/v1"
	ctlcorev1 "github.com/rancher/wrangler-api/pkg/generated/controllers/core/v1"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	appsv1type "k8s.io/client-go/kubernetes/typed/apps/v1"
	corev1type "k8s.io/client-go/kubernetes/typed/core/v1"
)

const cpNoScheduleTaintKey = "node-role.kubernetes.io/controlplane"

var cpNoScheduleTaint = &corev1.Taint{
	Key:    cpNoScheduleTaintKey,
	Effect: corev1.TaintEffectNoSchedule,
	Value:  "true",
}

func TestNodeHandler_OnChanged(t *testing.T) {
	type input struct {
		key         string
		node        *corev1.Node
		statefulset *appsv1.StatefulSet
		pods        []*corev1.Pod
	}
	type output struct {
		node               *corev1.Node
		statefulSetUpdated bool
		err                error
	}
	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "Node is not ready",
			given: input{
				key:         "test-node",
				statefulset: nil,
				node: &corev1.Node{
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   corev1.NodeReady,
								Status: corev1.ConditionFalse,
							},
						},
					},
				},
			},
			expected: output{
				node: &corev1.Node{
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   corev1.NodeReady,
								Status: corev1.ConditionFalse,
							},
						},
					},
				},
				statefulSetUpdated: false,
				err:                nil,
			},
		},
		{
			name: "Minio cluster is not present",
			given: input{
				key:         "test-node",
				statefulset: nil,
				node: &corev1.Node{
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   corev1.NodeReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				pods: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "minio-0",
							Labels: map[string]string{
								appLabelKey: minioName,
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node-1",
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodRunning,
						},
					},
				},
			},
			expected: output{
				node: &corev1.Node{
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   corev1.NodeReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				statefulSetUpdated: false,
				err:                nil,
			},
		},
		{
			name: "Minio is redeployed",
			given: input{
				key: "test-node",
				statefulset: &appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name: minioName,
					},
				},
				node: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node",
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   corev1.NodeReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				pods: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "minio-0",
							Labels: map[string]string{
								appLabelKey: minioName,
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node-1",
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodRunning,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "minio-1",
							Labels: map[string]string{
								appLabelKey: minioName,
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node-1",
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodRunning,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "minio-2",
							Labels: map[string]string{
								appLabelKey: minioName,
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node-1",
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodRunning,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "minio-3",
							Labels: map[string]string{
								appLabelKey: minioName,
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node-1",
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodRunning,
						},
					},
				},
			},
			expected: output{

				node: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node",
					},
					Status: corev1.NodeStatus{
						Conditions: []corev1.NodeCondition{
							{
								Type:   corev1.NodeReady,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				statefulSetUpdated: true,
				err:                nil,
			},
		},
		{
			name: "Test taints No Schedule",
			given: input{
				key: "test node taints",
				statefulset: &appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name: minioName,
					},
				},
				node: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "taint-node",
					},
					Spec: corev1.NodeSpec{
						Taints: []corev1.Taint{
							*cpNoScheduleTaint,
						},
					},
				},
				pods: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "minio-0",
							Labels: map[string]string{
								appLabelKey: minioName,
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node-1",
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodRunning,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "minio-1",
							Labels: map[string]string{
								appLabelKey: minioName,
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node-1",
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodRunning,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "minio-2",
							Labels: map[string]string{
								appLabelKey: minioName,
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node-1",
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodRunning,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "minio-3",
							Labels: map[string]string{
								appLabelKey: minioName,
							},
						},
						Spec: corev1.PodSpec{
							NodeName: "node-1",
						},
						Status: corev1.PodStatus{
							Phase: corev1.PodRunning,
						},
					},
				},
			},
			expected: output{
				node: &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "taint-node",
					},
					Spec: corev1.NodeSpec{
						Taints: []corev1.Taint{
							*cpNoScheduleTaint,
						},
					},
				},
				statefulSetUpdated: false,
				err:                nil,
			},
		},
	}
	for _, tc := range testCases {
		var clientset = fake.NewSimpleClientset()
		if tc.given.statefulset != nil {
			err := clientset.Tracker().Add(tc.given.statefulset)
			assert.Nil(t, err, "mock resource should add into fake controller tracker")
		}
		for _, pod := range tc.given.pods {
			err := clientset.Tracker().Add(pod)
			assert.Nil(t, err, "mock resource should add into fake controller tracker")
		}

		var handler = &Handler{
			podCache:         fakePodCache(clientset.CoreV1().Pods),
			statefulSets:     fakeStatefulSetClient(clientset.AppsV1().StatefulSets),
			statefulSetCache: fakeStatefulSetCache(clientset.AppsV1().StatefulSets),
		}
		var actual output
		actual.node, actual.err = handler.OnChanged(tc.given.key, tc.given.node)
		if tc.given.statefulset != nil {
			ss, err := handler.statefulSetCache.Get(tc.given.statefulset.Namespace, tc.given.statefulset.Name)
			assert.Nil(t, err)
			actual.statefulSetUpdated = ss.Spec.Template.Annotations[timestampAnnoKey] != tc.given.statefulset.Spec.Template.Annotations[timestampAnnoKey]
		}

		assert.Equal(t, tc.expected, actual, "case %q", tc.name)
	}
}

type fakePodCache func(string) corev1type.PodInterface

func (c fakePodCache) Get(namespace, name string) (*corev1.Pod, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}
func (c fakePodCache) List(namespace string, selector labels.Selector) ([]*corev1.Pod, error) {
	var result []*corev1.Pod
	list, err := c(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}
	for _, pod := range list.Items {
		result = append(result, &pod)
	}
	return result, err
}
func (c fakePodCache) AddIndexer(indexName string, indexer ctlcorev1.PodIndexer) {
	panic("implement me")
}
func (c fakePodCache) GetByIndex(indexName, key string) ([]*corev1.Pod, error) {
	panic("implement me")
}

type fakeStatefulSetClient func(string) appsv1type.StatefulSetInterface

func (c fakeStatefulSetClient) Update(ss *appsv1.StatefulSet) (*appsv1.StatefulSet, error) {
	return c(ss.Namespace).Update(context.TODO(), ss, metav1.UpdateOptions{})
}
func (c fakeStatefulSetClient) Get(namespace, name string, options metav1.GetOptions) (*appsv1.StatefulSet, error) {
	panic("implement me")
}
func (c fakeStatefulSetClient) Create(*appsv1.StatefulSet) (*appsv1.StatefulSet, error) {
	panic("implement me")
}
func (c fakeStatefulSetClient) UpdateStatus(*appsv1.StatefulSet) (*appsv1.StatefulSet, error) {
	panic("implement me")
}
func (c fakeStatefulSetClient) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	panic("implement me")
}
func (c fakeStatefulSetClient) List(namespace string, opts metav1.ListOptions) (*appsv1.StatefulSetList, error) {
	panic("implement me")
}
func (c fakeStatefulSetClient) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}
func (c fakeStatefulSetClient) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *appsv1.StatefulSet, err error) {
	panic("implement me")
}

type fakeStatefulSetCache func(string) appsv1type.StatefulSetInterface

func (c fakeStatefulSetCache) Get(namespace, name string) (*appsv1.StatefulSet, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}
func (c fakeStatefulSetCache) List(namespace string, selector labels.Selector) ([]*appsv1.StatefulSet, error) {
	panic("implement me")
}
func (c fakeStatefulSetCache) AddIndexer(indexName string, indexer ctlappsv1.StatefulSetIndexer) {
	panic("implement me")
}
func (c fakeStatefulSetCache) GetByIndex(indexName, key string) ([]*appsv1.StatefulSet, error) {
	panic("implement me")
}
