package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	apiserver "github.com/rancher/apiserver/pkg/server"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/wrangler/v3/pkg/schemas"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestActionAccessControlCanAction(t *testing.T) {
	tests := []struct {
		name         string
		allowed      bool
		actionExists bool
		wantErr      bool
	}{
		{name: "denies without resource update permission", allowed: false, actionExists: true, wantErr: true},
		{name: "allows action with resource update permission", allowed: true, actionExists: true},
		{name: "preserves unknown action rejection", allowed: true, actionExists: false, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewSimpleClientset()
			client.PrependReactor("create", "subjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
				review := action.(k8stesting.CreateAction).GetObject().(*authorizationv1.SubjectAccessReview)
				assert.Equal(t, "alice", review.Spec.User)
				assert.Equal(t, []string{"team-a"}, review.Spec.Groups)
				assert.Equal(t, "update", review.Spec.ResourceAttributes.Verb)
				assert.Equal(t, k8sschema.GroupVersionResource{Group: "example.io", Version: "v1alpha1", Resource: "widgets"}, k8sschema.GroupVersionResource{
					Group:    review.Spec.ResourceAttributes.Group,
					Version:  review.Spec.ResourceAttributes.Version,
					Resource: review.Spec.ResourceAttributes.Resource,
				})
				assert.Equal(t, "ns-a", review.Spec.ResourceAttributes.Namespace)
				assert.Equal(t, "widget-a", review.Spec.ResourceAttributes.Name)
				review.Status.Allowed = tc.allowed
				return true, review, nil
			})

			baseAccess := &apiserver.SchemaBasedAccess{}
			access := &actionAccessControl{
				AccessControl: baseAccess,
				sar:           client.AuthorizationV1().SubjectAccessReviews(),
			}
			req := httptest.NewRequest(http.MethodPost, "/v1/widgets/ns-a/widget-a?action=start", nil)
			req = req.WithContext(request.WithUser(req.Context(), &user.DefaultInfo{Name: "alice", Groups: []string{"team-a"}}))
			actionHandlers := map[string]http.Handler{}
			if tc.actionExists {
				actionHandlers["start"] = http.NotFoundHandler()
			}
			apiOp := &types.APIRequest{Request: req, Namespace: "ns-a", Name: "widget-a"}
			apiSchema := &types.APISchema{
				Schema: &schemas.Schema{ID: "example.io.widget", Attributes: map[string]interface{}{
					"group":    "example.io",
					"version":  "v1alpha1",
					"resource": "widgets",
				}},
				ActionHandlers: actionHandlers,
			}

			err := access.CanAction(apiOp, apiSchema, "start")
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestActionAccessControlUsesActionPermission(t *testing.T) {
	tests := []struct {
		name            string
		action          string
		wantVerb        string
		wantGroup       string
		wantResource    string
		wantSubresource string
		wantName        string
	}{
		{name: "start maps to the KubeVirt start subresource", action: "start", wantVerb: "update", wantGroup: "subresources.kubevirt.io", wantResource: "virtualmachines", wantSubresource: "start", wantName: "vm-a"},
		{name: "pause maps to the VMI pause subresource", action: "pause", wantVerb: "update", wantGroup: "subresources.kubevirt.io", wantResource: "virtualmachineinstances", wantSubresource: "pause", wantName: "vm-a"},
		{name: "backup maps to create on VM backups in the namespace", action: "backup", wantVerb: "create", wantGroup: "harvesterhci.io", wantResource: "virtualmachinebackups"},
		{name: "migrate maps to create on VMI migrations in the namespace", action: "migrate", wantVerb: "create", wantGroup: "kubevirt.io", wantResource: "virtualmachineinstancemigrations"},
		{name: "unmapped actions keep requiring update on the VM", action: "clone", wantVerb: "update", wantGroup: "kubevirt.io", wantResource: "virtualmachines", wantName: "vm-a"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewSimpleClientset()
			client.PrependReactor("create", "subjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
				review := action.(k8stesting.CreateAction).GetObject().(*authorizationv1.SubjectAccessReview)
				attrs := review.Spec.ResourceAttributes
				assert.Equal(t, tc.wantVerb, attrs.Verb)
				assert.Equal(t, tc.wantGroup, attrs.Group)
				assert.Equal(t, tc.wantResource, attrs.Resource)
				assert.Equal(t, tc.wantSubresource, attrs.Subresource)
				assert.Equal(t, "ns-a", attrs.Namespace)
				assert.Equal(t, tc.wantName, attrs.Name)
				review.Status.Allowed = true
				return true, review, nil
			})

			access := &actionAccessControl{
				AccessControl: &apiserver.SchemaBasedAccess{},
				sar:           client.AuthorizationV1().SubjectAccessReviews(),
			}
			req := httptest.NewRequest(http.MethodPost, "/v1/kubevirt.io.virtualmachines/ns-a/vm-a?action="+tc.action, nil)
			req = req.WithContext(request.WithUser(req.Context(), &user.DefaultInfo{Name: "alice"}))
			apiOp := &types.APIRequest{Request: req, Namespace: "ns-a", Name: "vm-a"}
			apiSchema := &types.APISchema{
				Schema: &schemas.Schema{ID: "kubevirt.io.virtualmachine", Attributes: map[string]interface{}{
					"group":    "kubevirt.io",
					"version":  "v1",
					"resource": "virtualmachines",
				}},
				ActionHandlers: map[string]http.Handler{tc.action: http.NotFoundHandler()},
			}

			require.NoError(t, access.CanAction(apiOp, apiSchema, tc.action))
		})
	}
}
