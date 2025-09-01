package accesscontrol

import (
	"sort"

	rbacv1controllers "github.com/rancher/wrangler/v3/pkg/generated/controllers/rbac/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/serviceaccount"
)

const (
	rbacGroup = rbacv1.GroupName
	All       = "*"

	groupKind      = rbacv1.GroupKind
	userKind       = rbacv1.UserKind
	svcAccountKind = rbacv1.ServiceAccountKind

	clusterRoleKind = "ClusterRole"
	roleKind        = "Role"
)

type policyRuleIndex struct {
	crCache             rbacv1controllers.ClusterRoleCache
	rCache              rbacv1controllers.RoleCache
	crbCache            rbacv1controllers.ClusterRoleBindingCache
	rbCache             rbacv1controllers.RoleBindingCache
	roleIndexKey        string
	clusterRoleIndexKey string
}

func newPolicyRuleIndex(user bool, rbac rbacv1controllers.Interface) *policyRuleIndex {
	key := groupKind
	if user {
		key = userKind
	}
	pi := &policyRuleIndex{
		crCache:             rbac.ClusterRole().Cache(),
		rCache:              rbac.Role().Cache(),
		crbCache:            rbac.ClusterRoleBinding().Cache(),
		rbCache:             rbac.RoleBinding().Cache(),
		clusterRoleIndexKey: "crb" + key,
		roleIndexKey:        "rb" + key,
	}

	pi.crbCache.AddIndexer(pi.clusterRoleIndexKey, clusterRoleBindingBySubjectIndexer(key))
	pi.rbCache.AddIndexer(pi.roleIndexKey, roleBindingBySubjectIndexer(key))

	return pi
}

func clusterRoleBindingBySubjectIndexer(kind string) func(crb *rbacv1.ClusterRoleBinding) ([]string, error) {
	return func(crb *rbacv1.ClusterRoleBinding) ([]string, error) {
		if crb.RoleRef.Kind != "ClusterRole" {
			return nil, nil
		}
		return indexSubjects(kind, crb.Subjects), nil
	}
}

func roleBindingBySubjectIndexer(key string) func(rb *rbacv1.RoleBinding) ([]string, error) {
	return func(rb *rbacv1.RoleBinding) ([]string, error) {
		return indexSubjects(key, rb.Subjects), nil
	}
}

func indexSubjects(kind string, subjects []rbacv1.Subject) []string {
	var result []string
	for _, subject := range subjects {
		if subjectIs(kind, subject) {
			result = append(result, subject.Name)
		} else if kind == userKind && subjectIsServiceAccount(subject) {
			// Index is for Users and this references a service account
			result = append(result, serviceaccount.MakeUsername(subject.Namespace, subject.Name))
		}
	}
	return result
}

// addAccess appends a set of PolicyRules to a given AccessSet
func addAccess(accessSet *AccessSet, namespace string, roleRef roleRef) {
	for _, rule := range roleRef.rules {
		if len(rule.Resources) > 0 {
			addResourceAccess(accessSet, namespace, rule)
		} else if roleRef.kind == clusterRoleKind {
			accessSet.AddNonResourceURLs(rule.Verbs, rule.NonResourceURLs)
		}
	}
}

func addResourceAccess(accessSet *AccessSet, namespace string, rule rbacv1.PolicyRule) {
	for _, group := range rule.APIGroups {
		for _, resource := range rule.Resources {
			names := rule.ResourceNames
			if len(names) == 0 {
				names = []string{All}
			}
			for _, resourceName := range names {
				for _, verb := range rule.Verbs {
					access := Access{
						Namespace:    namespace,
						ResourceName: resourceName,
					}

					// The first condition namespace != All is to determine if it is a RoleBinding.
					// The second and third conditions are to check if the resource is for "namespaces" in core group.
					// In kubernetes, rule are valid if they satisfy the following
					// 	- Should be `namespaces` GR
					//  - From RoleBindings in `namespace`
					//  - From Rule with  ResourceName `*`` or the `namespace` itself.
					// Ref: https://github.com/kubernetes/kubernetes/blob/master/staging/src/k8s.io/apiserver/pkg/endpoints/request/requestinfo.go#L194
					// If the ResourceName is `All` || namespace itself then only the current namespace is considered as Resourcename
					// In the case of Rolebinding for the resource "namespaces" in core group, access.Namespace
					// is set to All since namespace on the resource "namespaces" is not valid.
					if namespace != All && resource == "namespaces" && group == "" && (resourceName == All || resourceName == namespace) {
						access.Namespace = All
						access.ResourceName = namespace
					}

					accessSet.Add(verb,
						schema.GroupResource{
							Group:    group,
							Resource: resource,
						}, access)
				}
			}
		}
	}
}

func subjectIs(kind string, subject rbacv1.Subject) bool {
	return subject.APIGroup == rbacGroup && subject.Kind == kind
}

func subjectIsServiceAccount(subject rbacv1.Subject) bool {
	return subject.APIGroup == "" && subject.Kind == svcAccountKind && subject.Namespace != ""
}

// getRules obtain the actual Role or ClusterRole pointed at by a RoleRef, and returns PolicyRules and the resource version
func (p *policyRuleIndex) getRules(namespace string, roleRef rbacv1.RoleRef) ([]rbacv1.PolicyRule, string) {
	switch roleRef.Kind {
	case "ClusterRole":
		role, err := p.crCache.Get(roleRef.Name)
		if err != nil {
			return nil, ""
		}
		return role.Rules, role.ResourceVersion
	case "Role":
		role, err := p.rCache.Get(namespace, roleRef.Name)
		if err != nil {
			return nil, ""
		}
		return role.Rules, role.ResourceVersion
	}

	return nil, ""
}

func (p *policyRuleIndex) getClusterRoleBindings(subjectName string) []*rbacv1.ClusterRoleBinding {
	result, err := p.crbCache.GetByIndex(p.clusterRoleIndexKey, subjectName)
	if err != nil {
		return nil
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].UID < result[j].UID
	})
	return result
}

func (p *policyRuleIndex) getRoleBindings(subjectName string) []*rbacv1.RoleBinding {
	result, err := p.rbCache.GetByIndex(p.roleIndexKey, subjectName)
	if err != nil {
		return nil
	}
	sort.Slice(result, func(i, j int) bool {
		return string(result[i].UID) < string(result[j].UID)
	})
	return result
}

// getRoleRefs gathers rules from roles granted to a given subject through RoleBindings and ClusterRoleBindings
func (p *policyRuleIndex) getRoleRefs(subjectName string) subjectGrants {
	crbs := p.getClusterRoleBindings(subjectName)
	clusterRoleBindings := make([]roleRef, len(crbs))
	for x, crb := range crbs {
		rules, resourceVersion := p.getRules(All, crb.RoleRef)
		clusterRoleBindings[x] = roleRef{
			roleName:        crb.RoleRef.Name,
			resourceVersion: resourceVersion,
			rules:           rules,
			kind:            clusterRoleKind,
		}
	}

	rbs := p.getRoleBindings(subjectName)
	roleBindings := make([]roleRef, len(rbs))
	for x, rb := range rbs {
		rules, resourceVersion := p.getRules(rb.Namespace, rb.RoleRef)
		roleBindings[x] = roleRef{
			roleName:        rb.RoleRef.Name,
			namespace:       rb.Namespace,
			resourceVersion: resourceVersion,
			rules:           rules,
			kind:            roleKind,
		}
	}

	return subjectGrants{
		roleBindings:        roleBindings,
		clusterRoleBindings: clusterRoleBindings,
	}
}
