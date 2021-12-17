package subscribe

import (
	"encoding/json"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/wrangler/pkg/schemas/validation"
	"github.com/sirupsen/logrus"
)

var upgrader = websocket.Upgrader{
	HandshakeTimeout:  60 * time.Second,
	EnableCompression: true,
}

type Subscribe struct {
	Stop            bool   `json:"stop,omitempty"`
	ResourceType    string `json:"resourceType,omitempty"`
	ResourceVersion string `json:"resourceVersion,omitempty"`
	Namespace       string `json:"namespace,omitempty"`
	ID              string `json:"id,omitempty"`
	Selector        string `json:"selector,omitempty"`
}

func (s *Subscribe) key() string {
	return s.ResourceType + "/" + s.Namespace + "/" + s.ID + "/" + s.Selector
}

func NewHandler(getter SchemasGetter, serverVersion string) types.RequestListHandler {
	return func(apiOp *types.APIRequest) (types.APIObjectList, error) {
		return Handler(apiOp, getter, serverVersion)
	}
}

func Handler(apiOp *types.APIRequest, getter SchemasGetter, serverVersion string) (types.APIObjectList, error) {
	err := handler(apiOp, getter, serverVersion)
	if err != nil {
		logrus.Errorf("Error during subscribe %v", err)
	}
	return types.APIObjectList{}, validation.ErrComplete
}

func handler(apiOp *types.APIRequest, getter SchemasGetter, serverVersion string) error {
	c, err := upgrader.Upgrade(apiOp.Response, apiOp.Request, nil)
	if err != nil {
		return err
	}
	defer c.Close()

	watches := NewWatchSession(apiOp, getter)
	defer watches.Close()

	events := watches.Watch(c)
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	defer func() {
		// Ensure that events gets fully consumed
		go func() {
			for range events {
			}
		}()
	}()

	for {
		select {
		case event, ok := <-events:
			if !ok {
				return nil
			}
			if err := writeData(apiOp, getter, c, event); err != nil {
				return err
			}
		case <-t.C:
			if err := writeData(apiOp, getter, c, types.APIEvent{
				Name: "ping",
				Object: types.APIObject{
					Object: map[string]interface{}{"version": serverVersion},
				},
			}); err != nil {
				return err
			}
		}
	}
}

func writeData(apiOp *types.APIRequest, getter SchemasGetter, c *websocket.Conn, event types.APIEvent) error {
	event = MarshallObject(apiOp, getter, event)
	if event.Error != nil {
		event.Name = "resource.error"
		event.Data = map[string]interface{}{
			"error": event.Error.Error(),
		}
	}

	messageWriter, err := c.NextWriter(websocket.TextMessage)
	if err != nil {
		return err
	}
	defer messageWriter.Close()

	return json.NewEncoder(messageWriter).Encode(event)
}
