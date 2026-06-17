package middler

import "net/http"

type RequestMiddlerFunc func(req *http.Request) error

type RequestMiddler interface {
	ModifyRequest(req *http.Request) error
}

func (f RequestMiddlerFunc) ModifyRequest(req *http.Request) error {
	return f(req)
}

// WithRequestMiddlerFunc 是创建一个 RequestMiddler 的helper
// 如果我们只需要简单的逻辑，只关注闭包本身，则可以使用这个helper快速创建一个 RequestMiddler
func WithRequestMiddlerFunc(f RequestMiddlerFunc) RequestMiddler {
	return f
}
