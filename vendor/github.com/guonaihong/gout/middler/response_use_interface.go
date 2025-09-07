package middler

import (
	"net/http"
)

type ResponseMiddlerFunc func(response *http.Response) error

// ResponseMiddler 响应拦截器
type ResponseMiddler interface {
	ModifyResponse(response *http.Response) error
}

func (f ResponseMiddlerFunc) ModifyResponse(response *http.Response) error {
	return f(response)
}

// WithResponseMiddlerFunc 是创建一个 ResponseMiddler 的helper
// 如果我们只需要简单的逻辑，只关注闭包本身，则可以使用这个helper快速创建一个 ResponseMiddler
func WithResponseMiddlerFunc(f ResponseMiddlerFunc) ResponseMiddler {
	return f
}
