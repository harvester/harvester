package server

import (
	"net/http"

	"github.com/rancher/apiserver/pkg/builtin"
	"github.com/rancher/apiserver/pkg/handlers"
	"github.com/rancher/apiserver/pkg/parse"
	"github.com/rancher/apiserver/pkg/subscribe"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/apiserver/pkg/writer"
	"github.com/rancher/wrangler/pkg/schemas/validation"
)

type RequestHandler interface {
	http.Handler

	GetSchemas() *types.APISchemas
	Handle(apiOp *types.APIRequest)
}

type Server struct {
	ResponseWriters map[string]types.ResponseWriter
	Schemas         *types.APISchemas
	Defaults        Defaults
	AccessControl   types.AccessControl
	Parser          parse.Parser
	URLParser       parse.URLParser
}

type Defaults struct {
	ByIDHandler   types.RequestHandler
	ListHandler   types.RequestListHandler
	CreateHandler types.RequestHandler
	DeleteHandler types.RequestHandler
	UpdateHandler types.RequestHandler
	ErrorHandler  types.ErrorHandler
}

func DefaultAPIServer() *Server {
	s := &Server{
		Schemas: types.EmptyAPISchemas().MustAddSchemas(builtin.Schemas),
		ResponseWriters: map[string]types.ResponseWriter{
			"json": &writer.GzipWriter{
				ResponseWriter: &writer.EncodingResponseWriter{
					ContentType: "application/json",
					Encoder:     types.JSONEncoder,
				},
			},
			"html": &writer.GzipWriter{
				ResponseWriter: &writer.HTMLResponseWriter{
					EncodingResponseWriter: writer.EncodingResponseWriter{
						Encoder:     types.JSONEncoder,
						ContentType: "application/json",
					},
				},
			},
			"yaml": &writer.GzipWriter{
				ResponseWriter: &writer.EncodingResponseWriter{
					ContentType: "application/yaml",
					Encoder:     types.YAMLEncoder,
				},
			},
		},
		AccessControl: &SchemaBasedAccess{},
		Defaults: Defaults{
			ByIDHandler:   handlers.ByIDHandler,
			CreateHandler: handlers.CreateHandler,
			DeleteHandler: handlers.DeleteHandler,
			UpdateHandler: handlers.UpdateHandler,
			ListHandler:   handlers.ListHandler,
			ErrorHandler:  handlers.ErrorHandler,
		},
		Parser:    parse.Parse,
		URLParser: parse.MuxURLParser,
	}

	subscribe.Register(s.Schemas)
	return s
}

func (s *Server) setDefaults(ctx *types.APIRequest) {
	if ctx.ResponseWriter == nil {
		ctx.ResponseWriter = s.ResponseWriters[ctx.ResponseFormat]
		if ctx.ResponseWriter == nil {
			ctx.ResponseWriter = s.ResponseWriters["json"]
		}
	}

	ctx.AccessControl = s.AccessControl

	if ctx.Schemas == nil {
		ctx.Schemas = s.Schemas
	}
}

func (s *Server) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	s.Handle(&types.APIRequest{
		Request:  req,
		Response: rw,
	})
}

func (s *Server) Handle(apiOp *types.APIRequest) {
	s.handle(apiOp, s.Parser)
}

func (s *Server) handle(apiOp *types.APIRequest, parser parse.Parser) {
	if apiOp.Schemas == nil {
		apiOp.Schemas = s.Schemas
	}

	if err := parser(apiOp, parse.MuxURLParser); err != nil {
		// ensure defaults set so writer is assigned
		s.setDefaults(apiOp)
		s.handleError(apiOp, err)
		return
	}

	s.setDefaults(apiOp)

	var cloned *types.APISchemas
	for id, schema := range apiOp.Schemas.Schemas {
		if schema.RequestModifier == nil {
			continue
		}

		if cloned == nil {
			cloned = apiOp.Schemas.ShallowCopy()
		}

		schema := schema.DeepCopy()
		schema = schema.RequestModifier(apiOp, schema)
		cloned.Schemas[id] = schema
	}

	if cloned != nil {
		apiOp.Schemas = cloned
	}

	if apiOp.Schema != nil && apiOp.Schema.RequestModifier != nil {
		apiOp.Schema = apiOp.Schema.RequestModifier(apiOp, apiOp.Schema)
	}

	if code, data, err := s.handleOp(apiOp); err != nil {
		s.handleError(apiOp, err)
	} else if obj, ok := data.(types.APIObject); ok {
		apiOp.WriteResponse(code, obj)
	} else if list, ok := data.(types.APIObjectList); ok {
		apiOp.WriteResponseList(code, list)
	} else if code > http.StatusOK {
		apiOp.Response.WriteHeader(code)
	}
}

func (s *Server) handleOp(apiOp *types.APIRequest) (int, interface{}, error) {
	if err := CheckCSRF(apiOp); err != nil {
		return 0, nil, err
	}

	action, err := ValidateAction(apiOp)
	if err != nil {
		return 0, nil, err
	}

	if apiOp.Schema == nil {
		return http.StatusNotFound, nil, nil
	}

	if action != nil {
		return http.StatusOK, nil, handleAction(apiOp)
	}

	switch apiOp.Method {
	case http.MethodGet:
		if apiOp.Name == "" {
			data, err := handleList(apiOp, apiOp.Schema.ListHandler, s.Defaults.ListHandler)
			return http.StatusOK, data, err
		}
		data, err := handle(apiOp, apiOp.Schema.ByIDHandler, s.Defaults.ByIDHandler)
		return http.StatusOK, data, err
	case http.MethodPatch:
		fallthrough
	case http.MethodPut:
		data, err := handle(apiOp, apiOp.Schema.UpdateHandler, s.Defaults.UpdateHandler)
		return http.StatusOK, data, err
	case http.MethodPost:
		data, err := handle(apiOp, apiOp.Schema.CreateHandler, s.Defaults.CreateHandler)
		return http.StatusCreated, data, err
	case http.MethodDelete:
		data, err := handle(apiOp, apiOp.Schema.DeleteHandler, s.Defaults.DeleteHandler)
		return http.StatusOK, data, err
	}

	return http.StatusNotFound, nil, nil
}

func handleList(apiOp *types.APIRequest, custom types.RequestListHandler, handler types.RequestListHandler) (types.APIObjectList, error) {
	if custom != nil {
		return custom(apiOp)
	}
	return handler(apiOp)
}

func handle(apiOp *types.APIRequest, custom types.RequestHandler, handler types.RequestHandler) (types.APIObject, error) {
	if custom != nil {
		return custom(apiOp)
	}
	return handler(apiOp)
}

func handleAction(context *types.APIRequest) error {
	if err := context.AccessControl.CanAction(context, context.Schema, context.Action); err != nil {
		return err
	}
	if handler, ok := context.Schema.ActionHandlers[context.Action]; ok {
		handler.ServeHTTP(context.Response, context.Request)
		return validation.ErrComplete
	}
	return nil
}

func (s *Server) handleError(apiOp *types.APIRequest, err error) {
	if apiOp.Schema != nil && apiOp.Schema.ErrorHandler != nil {
		apiOp.Schema.ErrorHandler(apiOp, err)
	} else if s.Defaults.ErrorHandler != nil {
		s.Defaults.ErrorHandler(apiOp, err)
	}
}

func (s *Server) CustomAPIUIResponseWriter(cssURL, jsURL, version writer.StringGetter) {
	wi, ok := s.ResponseWriters["html"]
	if !ok {
		return
	}
	gw, ok := wi.(*writer.GzipWriter)
	if !ok {
		return
	}

	w, ok := gw.ResponseWriter.(*writer.HTMLResponseWriter)
	if !ok {
		return
	}
	w.CSSURL = cssURL
	w.JSURL = jsURL
	w.APIUIVersion = version
}
