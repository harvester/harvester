package accesscontrol

import (
	"fmt"
	"sort"

	rbacv1controllers "github.com/rancher/wrangler/v3/pkg/generated/controllers/rbac/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
			result = append(result, fmt.Sprintf("serviceaccount:%s:%s", subject.Namespace, subject.Name))
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
					accessSet.Add(verb,
						schema.GroupResource{
							Group:    group,
							Resource: resource,
						}, Access{
							Namespace:    namespace,
							ResourceName: resourceName,
						})
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
	var clusterRoleBindings []roleRef
	for _, crb := range p.getClusterRoleBindings(subjectName) {
		rules, resourceVersion := p.getRules(All, crb.RoleRef)
		clusterRoleBindings = append(clusterRoleBindings, roleRef{
			roleName:        crb.RoleRef.Name,
			resourceVersion: resourceVersion,
			rules:           rules,
			kind:            clusterRoleKind,
		})
	}

	var roleBindings []roleRef
	for _, rb := range p.getRoleBindings(subjectName) {
		rules, resourceVersion := p.getRules(rb.Namespace, rb.RoleRef)
		roleBindings = append(roleBindings, roleRef{
			roleName:        rb.RoleRef.Name,
			namespace:       rb.Namespace,
			resourceVersion: resourceVersion,
			rules:           rules,
			kind:            roleKind,
		})
	}

	return subjectGrants{
		roleBindings:        roleBindings,
		clusterRoleBindings: clusterRoleBindings,
	}
}
