package accesscontrol

import (
	"context"
	"slices"
	"sort"
	"time"

	v1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/rbac/v1"
	"golang.org/x/sync/singleflight"
	"k8s.io/apimachinery/pkg/util/cache"
	"k8s.io/apiserver/pkg/authentication/user"
)

//go:generate mockgen --build_flags=--mod=mod -package fake -destination fake/AccessSetLookup.go "github.com/rancher/steve/pkg/accesscontrol" AccessSetLookup

type AccessSetLookup interface {
	AccessFor(user user.Info) *AccessSet
	PurgeUserData(id string)
}

type policyRules interface {
	getRoleRefs(subjectName string) subjectGrants
}

// accessStoreCache is a subset of the methods implemented by LRUExpireCache
type accessStoreCache interface {
	Add(key interface{}, value interface{}, ttl time.Duration)
	Get(key interface{}) (interface{}, bool)
	Remove(key interface{})
}

type AccessStore struct {
	usersPolicyRules    policyRules
	groupsPolicyRules   policyRules
	cache               accessStoreCache
	concurrentAccessFor *singleflight.Group
}

func NewAccessStore(_ context.Context, cacheResults bool, rbac v1.Interface) *AccessStore {
	as := &AccessStore{
		usersPolicyRules:    newPolicyRuleIndex(true, rbac),
		groupsPolicyRules:   newPolicyRuleIndex(false, rbac),
		concurrentAccessFor: new(singleflight.Group),
	}
	if cacheResults {
		as.cache = cache.NewLRUExpireCache(50)
	}
	return as
}

func (l *AccessStore) AccessFor(user user.Info) *AccessSet {
	info := l.userGrantsFor(user)
	if l.cache == nil {
		return l.newAccessSet(info)
	}

	cacheKey := info.hash()

	res, _, _ := l.concurrentAccessFor.Do(cacheKey, func() (interface{}, error) {
		if val, ok := l.cache.Get(cacheKey); ok {
			as, _ := val.(*AccessSet)
			return as, nil
		}

		result := l.newAccessSet(info)
		result.ID = cacheKey
		l.cache.Add(cacheKey, result, 24*time.Hour)

		return result, nil
	})
	return res.(*AccessSet)
}

func (l *AccessStore) newAccessSet(info userGrants) *AccessSet {
	result := info.user.toAccessSet()
	for _, group := range info.groups {
		result.Merge(group.toAccessSet())
	}
	return result
}

func (l *AccessStore) PurgeUserData(id string) {
	l.cache.Remove(id)
}

// userGrantsFor retrieves all the access information for a user
func (l *AccessStore) userGrantsFor(user user.Info) userGrants {
	var res userGrants

	groups := slices.Clone(user.GetGroups())
	sort.Strings(groups)

	res.user = l.usersPolicyRules.getRoleRefs(user.GetName())
	for _, group := range groups {
		res.groups = append(res.groups, l.groupsPolicyRules.getRoleRefs(group))
	}

	return res
}
