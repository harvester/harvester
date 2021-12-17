package schemas

import (
	"context"
	"sync"
	"time"

	"github.com/rancher/apiserver/pkg/builtin"
	"k8s.io/apimachinery/pkg/api/equality"

	schemastore "github.com/rancher/apiserver/pkg/store/schema"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/accesscontrol"
	"github.com/rancher/steve/pkg/schema"
	"github.com/rancher/wrangler/pkg/broadcast"
	"github.com/rancher/wrangler/pkg/schemas/validation"
	"github.com/sirupsen/logrus"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"
)

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

type Store struct {
	types.Store

	asl                accesscontrol.AccessSetLookup
	sf                 schema.Factory
	schemaChangeNotify func(context.Context) (chan interface{}, error)
}

func (s *Store) Watch(apiOp *types.APIRequest, schema *types.APISchema, w types.WatchRequest) (chan types.APIEvent, error) {
	user, ok := request.UserFrom(apiOp.Request.Context())
	if !ok {
		return nil, validation.Unauthorized
	}

	wg := sync.WaitGroup{}
	wg.Add(2)
	result := make(chan types.APIEvent)

	go func() {
		wg.Wait()
		close(result)
	}()

	go func() {
		defer wg.Done()
		c, err := s.schemaChangeNotify(apiOp.Context())
		if err != nil {
			return
		}
		schemas, err := s.sf.Schemas(user)
		if err != nil {
			logrus.Errorf("failed to generate schemas for user %v: %v", user, err)
			return
		}
		for range c {
			schemas = s.sendSchemas(result, apiOp, user, schemas)
		}
	}()

	go func() {
		defer wg.Done()
		schemas, err := s.sf.Schemas(user)
		if err != nil {
			logrus.Errorf("failed to generate schemas for notify user %v: %v", user, err)
			return
		}
		for range s.userChangeNotify(apiOp.Context(), user) {
			schemas = s.sendSchemas(result, apiOp, user, schemas)
		}
	}()

	return result, nil
}

func (s *Store) sendSchemas(result chan types.APIEvent, apiOp *types.APIRequest, user user.Info, oldSchemas *types.APISchemas) *types.APISchemas {
	schemas, err := s.sf.Schemas(user)
	if err != nil {
		logrus.Errorf("failed to get schemas for %v: %v", user, err)
		return oldSchemas
	}

	inNewSchemas := map[string]bool{}
	for _, apiObject := range schemastore.FilterSchemas(apiOp, schemas.Schemas).Objects {
		inNewSchemas[apiObject.ID] = true
		eventName := types.ChangeAPIEvent
		if oldSchema := oldSchemas.LookupSchema(apiObject.ID); oldSchema == nil {
			eventName = types.CreateAPIEvent
		} else {
			newSchemaCopy := apiObject.Object.(*types.APISchema).Schema.DeepCopy()
			oldSchemaCopy := oldSchema.Schema.DeepCopy()
			newSchemaCopy.Mapper = nil
			oldSchemaCopy.Mapper = nil
			if equality.Semantic.DeepEqual(newSchemaCopy, oldSchemaCopy) {
				continue
			}
		}
		result <- types.APIEvent{
			Name:         eventName,
			ResourceType: "schema",
			Object:       apiObject,
		}
	}

	for _, oldSchema := range schemastore.FilterSchemas(apiOp, oldSchemas.Schemas).Objects {
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
