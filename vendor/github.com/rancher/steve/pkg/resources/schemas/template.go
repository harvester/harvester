// Package schemas handles streaming schema updates and changes.
package schemas

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rancher/apiserver/pkg/builtin"
	schemastore "github.com/rancher/apiserver/pkg/store/schema"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/accesscontrol"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/wrangler/v3/pkg/broadcast"
	"github.com/rancher/wrangler/v3/pkg/schemas/validation"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"
)

// SetupWatcher create a new schema.Store for tracking schema changes
func SetupWatcher(ctx context.Context, schemas *types.APISchemas, asl accesscontrol.AccessSetLookup, factory schema.Factory) {
	// one instance shared with all stores
	notifier := schemaChangeNotifier(ctx, factory)

	schema := builtin.Schema
	schema.Store = &Store{
		Store:              schema.Store,
		asl:                asl,
		sf:                 factory,
		schemaChangeNotify: notifier,
	}

	schemas.AddSchema(schema)
}

// Store hold information for watching updates to schemas
type Store struct {
	types.Store

	asl                accesscontrol.AccessSetLookup
	sf                 schema.Factory
	schemaChangeNotify func(context.Context) (chan interface{}, error)
}

// Watch will return a APIevent channel that tracks changes to schemas for a user in a given APIRequest.
// Changes will be returned until Done is closed on the context in the given APIRequest.
func (s *Store) Watch(apiOp *types.APIRequest, _ *types.APISchema, _ types.WatchRequest) (chan types.APIEvent, error) {
	user, ok := request.UserFrom(apiOp.Request.Context())
	if !ok {
		return nil, validation.Unauthorized
	}

	wg := sync.WaitGroup{}
	wg.Add(1)
	result := make(chan types.APIEvent)

	go func() {
		wg.Wait()
		close(result)
	}()

	schemas, err := s.sf.Schemas(user)
	if err != nil {
		return nil, fmt.Errorf("failed to generate schemas for user '%v': %w", user, err)
	}

	// Create child contexts that allows us to cancel both change notifications routines.
	notifyCtx, notifyCancel := context.WithCancel(apiOp.Context())

	schemaChangeSignal, err := s.schemaChangeNotify(notifyCtx)
	if err != nil {
		notifyCancel()
		return nil, fmt.Errorf("failed to start schema change notifications: %w", err)
	}

	userChangeSignal := s.userChangeNotify(notifyCtx, user)

	go func() {
		defer notifyCancel()
		defer wg.Done()

		// For each change notification send schema updates onto the result channel.
		for {
			select {
			case _, ok := <-schemaChangeSignal:
				if !ok {
					return
				}
			case _, ok := <-userChangeSignal:
				if !ok {
					return
				}
			}
			schemas = s.sendSchemas(result, apiOp, user, schemas)
		}
	}()

	return result, nil
}

// sendSchemas will send APIEvents onto the provided result channel based on detected changes in the schemas for the provided users.
func (s *Store) sendSchemas(result chan types.APIEvent, apiOp *types.APIRequest, user user.Info, oldSchemas *types.APISchemas) *types.APISchemas {
	// get the current schemas for a user
	schemas, err := s.sf.Schemas(user)
	if err != nil {
		logrus.Errorf("failed to get schemas for %v: %v", user, err)
		return oldSchemas
	}

	inNewSchemas := map[string]bool{}

	// Convert the schemas for the given user to a flat list of APIObjects.
	apiObjects := schemastore.FilterSchemas(apiOp, schemas.Schemas).Objects
	for i := range apiObjects {
		apiObject := apiObjects[i]
		inNewSchemas[apiObject.ID] = true
		eventName := types.ChangeAPIEvent

		// Check to see if the schema represented by the current APIObject exist in the oldSchemas.
		if oldSchema := oldSchemas.LookupSchema(apiObject.ID); oldSchema == nil {
			eventName = types.CreateAPIEvent
		} else {
			newSchemaCopy := apiObject.Object.(*types.APISchema).Schema.DeepCopy()
			oldSchemaCopy := oldSchema.Schema.DeepCopy()
			newSchemaCopy.Mapper = nil
			oldSchemaCopy.Mapper = nil

			// APIObjects are intentionally stripped of access information. Thus we will remove the field when comparing changes.
			delete(oldSchemaCopy.Attributes, "access")
			if equality.Semantic.DeepEqual(newSchemaCopy, oldSchemaCopy) {
				continue
			}
		}

		// Send the new or modified schema as an APIObject on the APIEvent channel.
		result <- types.APIEvent{
			Name:         eventName,
			ResourceType: "schema",
			Object:       apiObject,
		}
	}

	// Identify all of the oldSchema APIObjects that have been removed and send Remove APIEvents.
	oldSchemaObjs := schemastore.FilterSchemas(apiOp, oldSchemas.Schemas).Objects
	for i := range oldSchemaObjs {
		oldSchema := oldSchemaObjs[i]
		if inNewSchemas[oldSchema.ID] {
			continue
		}
		result <- types.APIEvent{
			Name:         types.RemoveAPIEvent,
			ResourceType: "schema",
			Object:       oldSchema,
		}
	}

	return schemas
}

// userChangeNotify gets the provided users AccessSet every 2 seconds.
// If the AccessSet has changed the caller is notified via an empty struct sent on the returned channel.
// If the given context is finished then the returned channel will be closed.
func (s *Store) userChangeNotify(ctx context.Context, user user.Info) chan interface{} {
	as := s.asl.AccessFor(user)
	result := make(chan interface{})
	go func() {
		defer close(result)
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}

			newAS := s.asl.AccessFor(user)
			if newAS.ID != as.ID {
				result <- struct{}{}
				as = newAS
			}
		}
	}()

	return result
}

// schemaChangeNotifier returns a channel that is used to signal OnChange was called for the provided factory.
func schemaChangeNotifier(ctx context.Context, factory schema.Factory) func(ctx context.Context) (chan interface{}, error) {
	notify := make(chan interface{})
	bcast := &broadcast.Broadcaster{}
	factory.OnChange(ctx, func() {
		select {
		case notify <- struct{}{}:
		default:
		}
	})
	return func(ctx context.Context) (chan interface{}, error) {
		return bcast.Subscribe(ctx, func() (chan interface{}, error) {
			return notify, nil
		})
	}
}
