package parse

import (
	"net/http"

	"github.com/rancher/apiserver/pkg/types"
)

type Vars struct {
	Type      string
	Name      string
	Namespace string
	Link      string
	Prefix    string
	Action    string
}

func Set(v Vars) func(*http.Request) bool {
	return func(request *http.Request) bool {
		if v.Type != "" {
			request.SetPathValue("type", v.Type)
		}
		if v.Name != "" {
			request.SetPathValue("name", v.Name)
		}
		if v.Link != "" {
			request.SetPathValue("link", v.Link)
		}
		if v.Prefix != "" {
			request.SetPathValue("prefix", v.Prefix)
		}
		if v.Action != "" {
			request.SetPathValue("action", v.Action)
		}
		if v.Namespace != "" {
			request.SetPathValue("namespace", v.Namespace)
		}
		return true
	}
}

func StandardURLParser(rw http.ResponseWriter, req *http.Request, schemas *types.APISchemas) (ParsedURL, error) {
	url := ParsedURL{
		Type:      req.PathValue("type"),
		Name:      req.PathValue("name"),
		Namespace: req.PathValue("namespace"),
		Prefix:    req.PathValue("prefix"),
		Method:    req.Method,
		Query:     req.URL.Query(),
	}

	// 'action' and 'link' could be path variables or query string parameters.
	// We check PathValue first, then fallback to URL queries.
	url.Action = req.PathValue("action")
	if url.Action == "" {
		url.Action = req.URL.Query().Get("action")
	}

	url.Link = req.PathValue("link")
	if url.Link == "" {
		url.Link = req.URL.Query().Get("link")
	}

	return url, nil
}
