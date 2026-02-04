package subscribe

import (
	"io"

	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/apiserver/pkg/writer"
)

type Converter struct {
	writer.EncodingResponseWriter
	apiOp *types.APIRequest
	obj   interface{}
}

func MarshallObject(apiOp *types.APIRequest, getter SchemasGetter, event types.APIEvent) types.APIEvent {
	if event.Error != nil {
		return event
	}

	apiOp = apiOp.Clone()
	apiOp.Schemas = getter(apiOp)
	schema := apiOp.Schemas.LookupSchema(event.Object.Type)
	if schema != nil {
		apiOp.Schema = schema
	}
	data, err := newConverter(apiOp).ToAPIObject(event.Object)
	if err != nil {
		event.Error = err
		return event
	}

	event.Data = data
	return event
}

func newConverter(apiOp *types.APIRequest) *Converter {
	c := &Converter{
		apiOp: apiOp,
	}
	c.EncodingResponseWriter = writer.EncodingResponseWriter{
		ContentType: "application/json",
		Encoder:     c.Encoder,
	}
	return c
}

func (c *Converter) ToAPIObject(data types.APIObject) (interface{}, error) {
	c.obj = nil
	if err := c.Body(c.apiOp, nil, data); err != nil {
		return types.APIObject{}, err
	}
	return c.obj, nil
}

func (c *Converter) Encoder(_ io.Writer, obj interface{}) error {
	c.obj = obj
	return nil
}
