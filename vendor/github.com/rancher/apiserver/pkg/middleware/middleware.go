package middleware

import (
	"net/http"
)

type Chain []func(http.Handler) http.Handler

func (m Chain) Handler(handler http.Handler) http.Handler {
	rtn := handler
	for i := len(m) - 1; i >= 0; i-- {
		w := m[i]
		rtn = w(rtn)
	}
	return rtn
}
