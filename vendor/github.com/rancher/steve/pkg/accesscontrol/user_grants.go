package accesscontrol

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"

	rbacv1 "k8s.io/api/rbac/v1"
)

// userGrants is a complete snapshot of all rules granted to a user, including those through groups memberships
type userGrants struct {
	user   subjectGrants
	groups []subjectGrants
}

// subjectGrants defines role references granted to a given subject through RoleBindings and ClusterRoleBindings
type subjectGrants struct {
	roleBindings        []roleRef
	clusterRoleBindings []roleRef
}

// roleRef contains information from a Role or ClusterRole
type roleRef struct {
	namespace, roleName, resourceVersion, kind string
	rules                                      []rbacv1.PolicyRule
}

// hash calculates a unique identifier from all the grants for a user
func (u userGrants) hash() string {
	d := sha256.New()
	u.user.writeTo(d)
	for _, group := range u.groups {
		group.writeTo(d)
	}
	return hex.EncodeToString(d.Sum(nil))
}

// writeTo appends a subject's grants information to a given hash
func (b subjectGrants) writeTo(digest hash.Hash) {
	for _, rb := range b.roleBindings {
		rb.writeTo(digest)
	}
	for _, crb := range b.clusterRoleBindings {
		crb.writeTo(digest)
	}
}

// toAccessSet produces a new AccessSet from the rules in the inner roles references
func (b subjectGrants) toAccessSet() *AccessSet {
	result := new(AccessSet)

	for _, binding := range b.roleBindings {
		addAccess(result, binding.namespace, binding)
	}

	for _, binding := range b.clusterRoleBindings {
		addAccess(result, All, binding)
	}

	return result
}

// writeTo appends a single role information to a given hash
func (r roleRef) writeTo(digest hash.Hash) {
	digest.Write([]byte(r.roleName))
	if r.namespace != "" {
		digest.Write([]byte(r.namespace))
	}
	digest.Write([]byte(r.resourceVersion))
}
