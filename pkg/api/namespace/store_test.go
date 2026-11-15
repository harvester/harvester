package namespace

import (
	"net/url"
	"testing"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/harvester/harvester/pkg/util/fakeclients"
)

type recordingStore struct {
	types.Store
	listCalls int
}

func (s *recordingStore) List(_ *types.APIRequest, _ *types.APISchema) (types.APIObjectList, error) {
	s.listCalls++
	return types.APIObjectList{Count: 42}, nil
}

func TestStoreList(t *testing.T) {
	tests := []struct {
		name          string
		query         url.Values
		expectCount   int
		expectInner   int
		expectedNames []string
	}{
		{
			name:        "no link is delegated to the wrapped store",
			query:       url.Values{},
			expectCount: 42,
			expectInner: 1,
		},
		{
			name:        "unknown link is delegated to the wrapped store",
			query:       url.Values{queryParamLink: {"unknown"}},
			expectCount: 42,
			expectInner: 1,
		},
		{
			name:          "supportbundle link is answered from the cache",
			query:         url.Values{queryParamLink: {linkTypeSupportBundle}},
			expectCount:   2,
			expectInner:   0,
			expectedNames: []string{"harvester-system", "longhorn-system"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clientset := k8sfake.NewSimpleClientset(
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "harvester-system"}},
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "longhorn-system"}},
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "not-in-the-default-list"}},
			)

			inner := &recordingStore{}
			store := &Store{
				Store:   inner,
				nsCache: fakeclients.NamespaceCache(clientset.CoreV1().Namespaces),
			}

			list, err := store.List(&types.APIRequest{Query: tc.query}, nil)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectCount, list.Count)
			assert.Equal(t, tc.expectInner, inner.listCalls)

			names := make([]string, 0, len(list.Objects))
			for _, obj := range list.Objects {
				names = append(names, obj.ID)
			}
			if tc.expectedNames == nil {
				assert.Empty(t, names)
			} else {
				assert.ElementsMatch(t, tc.expectedNames, names)
			}
		})
	}
}
