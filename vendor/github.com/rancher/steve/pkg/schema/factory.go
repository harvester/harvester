package schema

//go:generate go tool -modfile ../../gotools/mockgen/go.mod mockgen --build_flags=--mod=mod -package fake -destination fake/factory.go "github.com/rancher/steve/pkg/schema" Factory
import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/rancher/apiserver/pkg/builtin"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/accesscontrol"
	"github.com/rancher/steve/pkg/attributes"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/user"
)

type Factory interface {
	Schemas(user user.Info) (*types.APISchemas, error)
	ByGVR(gvr schema.GroupVersionResource) string
	ByGVK(gvr schema.GroupVersionKind) string
	OnChange(ctx context.Context, cb func())
	AddTemplate(template ...Template)
}

func newSchemas() (*types.APISchemas, error) {
	apiSchemas := types.EmptyAPISchemas()
	if err := apiSchemas.AddSchemas(builtin.Schemas); err != nil {
		return nil, err
	}

	return apiSchemas, nil
}

func (c *Collection) Schemas(user user.Info) (*types.APISchemas, error) {
	access := c.as.AccessFor(user)
	c.removeOldRecords(access, user)
	val, ok := c.cache.Get(access.ID)
	if ok {
		schemas, _ := val.(*types.APISchemas)
		return schemas, nil
	}

	schemas, err := c.schemasForSubject(access)
	if err != nil {
		return nil, err
	}
	c.addToCache(access, user, schemas)
	return schemas, nil
}

func (c *Collection) removeOldRecords(access *accesscontrol.AccessSet, user user.Info) {
	current, ok := c.userCache.Get(user.GetName())
	if ok {
		currentID, cOk := current.(string)
		if cOk && currentID != access.ID {
			// we only want to keep around one record per user. If our current access record is invalid, purge the
			//record of it from the cache, so we don't keep duplicates
			c.purgeUserRecords(currentID)
			c.userCache.Remove(user.GetName())
		}
	}
}

func (c *Collection) addToCache(access *accesscontrol.AccessSet, user user.Info, schemas *types.APISchemas) {
	c.cache.Add(access.ID, schemas, 24*time.Hour)
	c.userCache.Add(user.GetName(), access.ID, 24*time.Hour)
}

// PurgeUserRecords removes a record from the backing LRU cache before expiry
func (c *Collection) purgeUserRecords(id string) {
	c.cache.Remove(id)
	c.as.PurgeUserData(id)
}

func (c *Collection) schemasForSubject(access *accesscontrol.AccessSet) (*types.APISchemas, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	result, err := newSchemas()
	if err != nil {
		return nil, err
	}

	if err := result.AddSchemas(c.baseSchema); err != nil {
		return nil, err
	}

	for _, s := range c.schemas {
		gr := attributes.GR(s)

		if gr.Resource == "" {
			if err := result.AddSchema(*s); err != nil {
				return nil, err
			}
			continue
		}

		verbs := attributes.Verbs(s)
		verbAccess := accesscontrol.AccessListByVerb{}

		for _, verb := range verbs {
			a := access.AccessListFor(verb, gr)
			if !attributes.Namespaced(s) {
				// trim out bad data where we are granted namespaced access to cluster scoped object
				filtered := accesscontrol.AccessList{}
				for _, ac := range a {
					if ac.Namespace == accesscontrol.All {
						filtered = append(filtered, ac)
					}
				}
				a = filtered
			}
			if len(a) > 0 {
				verbAccess[verb] = a
			}
		}

		mustAllowList := false
		if len(verbAccess) == 0 {
			if gr.Group == "" && gr.Resource == "namespaces" {
				var accessList accesscontrol.AccessList
				for _, ns := range access.Namespaces() {
					accessList = append(accessList, accesscontrol.Access{
						Namespace:    accesscontrol.All,
						ResourceName: ns,
					})
				}
				verbAccess["get"] = accessList
				verbAccess["watch"] = accessList
				if len(accessList) == 0 {
					// make sure we add GET to collection later
					mustAllowList = true
				}
			}
		}

		s = s.DeepCopy()
		attributes.SetAccess(s, verbAccess)
		sm := newSchemaMethodsFromSchema(s)

		allowed := func(method string) string {
			if attributes.DisallowMethods(s)[method] {
				return "blocked-" + method
			}
			return method
		}

		if mustAllowList {
			sm.addCollection(http.MethodGet)
		}

		if verbAccess.AnyVerb("list", "get") {
			sm.addResource(allowed(http.MethodGet))
			sm.addCollection(allowed(http.MethodGet))
		}
		if verbAccess.AnyVerb("delete") {
			sm.addResource(allowed(http.MethodDelete))
		}
		if verbAccess.AnyVerb("update") {
			sm.addResource(allowed(http.MethodPut))
			sm.addResource(allowed(http.MethodPatch))
		}
		if verbAccess.AnyVerb("create") {
			sm.addCollection(allowed(http.MethodPost))
		}
		if verbAccess.AnyVerb("patch") {
			sm.addResource(allowed(http.MethodPatch))
		}

		sm.applyToSchema(s)

		if len(s.CollectionMethods) == 0 && len(s.ResourceMethods) == 0 {
			continue
		}

		if err := result.AddSchema(*s); err != nil {
			return nil, err
		}
	}

	accesscontrol.SetAccessSetAttribute(result, access)
	return result, nil
}

func (c *Collection) defaultStore() types.Store {
	templates := c.templates[""]
	if len(templates) > 0 {
		return templates[0].Store
	}
	return nil
}

func (c *Collection) applyTemplates(schema *types.APISchema) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	templates := [][]*Template{
		c.templates[schema.ID],
		c.templates[fmt.Sprintf("%s/%s", attributes.Group(schema), attributes.Kind(schema))],
		c.templates[""],
	}

	for _, templates := range templates {
		for _, t := range templates {
			if t == nil {
				continue
			}
			if schema.Formatter == nil {
				schema.Formatter = t.Formatter
			} else if t.Formatter != nil {
				schema.Formatter = types.FormatterChain(t.Formatter, schema.Formatter)
			}
			if schema.Store == nil {
				if t.StoreFactory == nil {
					schema.Store = t.Store
				} else {
					schema.Store = t.StoreFactory(c.defaultStore())
				}
			}
			if t.Customize != nil {
				t.Customize(schema)
			}
		}
	}
}

// helper to manage collection/resource methods uniformly
type schemaMethods struct {
	collection map[string]struct{}
	resource   map[string]struct{}
}

func newSchemaMethodsFromSchema(s *types.APISchema) schemaMethods {
	sm := schemaMethods{
		collection: make(map[string]struct{}, len(s.CollectionMethods)),
		resource:   make(map[string]struct{}, len(s.ResourceMethods)),
	}
	for _, m := range s.CollectionMethods {
		sm.collection[m] = struct{}{}
	}
	for _, m := range s.ResourceMethods {
		sm.resource[m] = struct{}{}
	}
	return sm
}

func (sm *schemaMethods) addCollection(method string) {
	if method == "" {
		return
	}
	sm.collection[method] = struct{}{}
}

func (sm *schemaMethods) addResource(method string) {
	if method == "" {
		return
	}
	sm.resource[method] = struct{}{}
}

func (sm *schemaMethods) applyToSchema(s *types.APISchema) {
	s.CollectionMethods = make([]string, 0, len(sm.collection))
	for m := range sm.collection {
		s.CollectionMethods = append(s.CollectionMethods, m)
	}
	s.ResourceMethods = make([]string, 0, len(sm.resource))
	for m := range sm.resource {
		s.ResourceMethods = append(s.ResourceMethods, m)
	}
}
