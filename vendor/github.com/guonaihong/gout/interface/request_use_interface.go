package dataflow

import "net/http"

type RequestMiddler interface {
	ModifyRequest(req *http.Request) error
}
