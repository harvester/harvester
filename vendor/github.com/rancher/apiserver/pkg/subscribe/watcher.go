package subscribe

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/rancher/apiserver/pkg/types"
)

type WatchSession struct {
	sync.Mutex

	apiOp    *types.APIRequest
	getter   SchemasGetter
	watchers map[string]func()
	wg       sync.WaitGroup
	ctx      context.Context
	cancel   func()
}

func (s *WatchSession) stop(sub Subscribe, resp chan<- types.APIEvent) {
	s.Lock()
	defer s.Unlock()
	if cancel, ok := s.watchers[sub.key()]; ok {
		cancel()
		resp <- types.APIEvent{
			Name:         "resource.stop",
			ResourceType: sub.ResourceType,
			Namespace:    sub.Namespace,
			ID:           sub.ID,
			Selector:     sub.Selector,
		}
	}
	delete(s.watchers, sub.key())
}

func (s *WatchSession) add(sub Subscribe, resp chan<- types.APIEvent) {
	s.Lock()
	defer s.Unlock()

	ctx, cancel := context.WithCancel(s.ctx)
	s.watchers[sub.key()] = cancel

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer s.stop(sub, resp)

		if err := s.stream(ctx, sub, resp); err != nil {
			sendErr(resp, err, sub)
		}
	}()
}

func (s *WatchSession) stream(ctx context.Context, sub Subscribe, result chan<- types.APIEvent) error {
	schemas := s.getter(s.apiOp)
	schema := schemas.LookupSchema(sub.ResourceType)
	if schema == nil {
		return fmt.Errorf("failed to find schema %s", sub.ResourceType)
	} else if schema.Store == nil {
		return fmt.Errorf("schema %s does not support watching", sub.ResourceType)
	}

	if err := s.apiOp.AccessControl.CanWatch(s.apiOp, schema); err != nil {
		return err
	}

	apiOp := s.apiOp.Clone().WithContext(ctx)
	apiOp.Namespace = sub.Namespace
	apiOp.Schemas = schemas
	c, err := schema.Store.Watch(apiOp, schema, types.WatchRequest{
		Revision: sub.ResourceVersion,
		ID:       sub.ID,
		Selector: sub.Selector,
	})
	if err != nil {
		return err
	}

	result <- types.APIEvent{
		Name:         "resource.start",
		ResourceType: sub.ResourceType,
		ID:           sub.ID,
		Selector:     sub.Selector,
	}

	if c == nil {
		<-s.apiOp.Context().Done()
	} else {
		for event := range c {
			if event.Error == nil {
				event.ID = sub.ID
				event.Selector = sub.Selector
				select {
				case result <- event:
				default:
					// handle slow consumer
					go func() {
						for range c {
							// continue to drain until close
						}
					}()
					return nil
				}
			} else {
				sendErr(result, event.Error, sub)
			}
		}
	}

	return nil
}

func NewWatchSession(apiOp *types.APIRequest, getter SchemasGetter) *WatchSession {
	ws := &WatchSession{
		apiOp:    apiOp,
		getter:   getter,
		watchers: map[string]func(){},
	}

	ws.ctx, ws.cancel = context.WithCancel(apiOp.Request.Context())
	return ws
}

func (s *WatchSession) Watch(conn *websocket.Conn) <-chan types.APIEvent {
	result := make(chan types.APIEvent, 100)
	go func() {
		defer close(result)

		if err := s.watch(conn, result); err != nil {
			sendErr(result, err, Subscribe{})
		}
	}()
	return result
}

func (s *WatchSession) Close() {
	s.cancel()
	s.wg.Wait()
}

func (s *WatchSession) watch(conn *websocket.Conn, resp chan types.APIEvent) error {
	defer s.wg.Wait()
	defer s.cancel()

	for {
		_, r, err := conn.NextReader()
		if err != nil {
			return err
		}

		var sub Subscribe

		if err := json.NewDecoder(r).Decode(&sub); err != nil {
			sendErr(resp, err, Subscribe{})
			continue
		}

		if sub.Stop {
			s.stop(sub, resp)
		} else {
			s.Lock()
			_, ok := s.watchers[sub.key()]
			s.Unlock()
			if !ok {
				s.add(sub, resp)
			}
		}
	}
}

func sendErr(resp chan<- types.APIEvent, err error, sub Subscribe) {
	resp <- types.APIEvent{
		ResourceType: sub.ResourceType,
		Namespace:    sub.Namespace,
		ID:           sub.ID,
		Selector:     sub.Selector,
		Error:        err,
	}
}
