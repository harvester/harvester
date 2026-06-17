package middler

import "net/http"

type Do interface {
	Do(*http.Request) (*http.Response, error)
}
