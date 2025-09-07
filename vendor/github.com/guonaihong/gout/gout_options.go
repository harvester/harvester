package gout

import (
	"crypto/tls"
	"net/http"
	"time"

	"github.com/guonaihong/gout/hcutil"
	"github.com/guonaihong/gout/setting"
)

type options struct {
	hc *http.Client
	setting.Setting
	err error
}

type Option interface {
	apply(*options)
}

// 1.start
type insecureSkipVerifyOption bool

func (i insecureSkipVerifyOption) apply(opts *options) {

	if opts.hc.Transport == nil {
		opts.hc.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
		return
	}
	opts.hc.Transport.(*http.Transport).TLSClientConfig = &tls.Config{
		InsecureSkipVerify: true,
	}

}

// 1.忽略ssl验证
func WithInsecureSkipVerify() Option {
	b := true
	return insecureSkipVerifyOption(b)
}

// 2. start
type client http.Client

func (c *client) apply(opts *options) {
	opts.hc = (*http.Client)(c)
}

// 2.自定义http.Client
func WithClient(c *http.Client) Option {
	return (*client)(c)
}

// 3.start
type close3xx struct{}

func (c close3xx) apply(opts *options) {
	opts.hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
}

// 3.关闭3xx自动跳转
func WithClose3xxJump() Option {
	return close3xx{}
}

// 4.timeout
type timeout time.Duration

func WithTimeout(t time.Duration) Option {
	return (*timeout)(&t)
}

func (t *timeout) apply(opts *options) {
	opts.SetTimeout(time.Duration(*t))
}

// 5. 设置代理
type proxy string

func WithProxy(p string) Option {
	return (*proxy)(&p)
}

func (p *proxy) apply(opts *options) {
	if opts.hc == nil {
		opts.hc = &http.Client{}
	}

	opts.err = hcutil.SetProxy(opts.hc, string(*p))
}

// 6. 设置socks5代理
type socks5 string

func WithSocks5(s string) Option {
	return (*socks5)(&s)
}

func (s *socks5) apply(opts *options) {
	if opts.hc == nil {
		opts.hc = &http.Client{}
	}

	opts.err = hcutil.SetSOCKS5(opts.hc, string(*s))
}

// 7. 设置unix socket
type unixSocket string

func WithUnixSocket(u string) Option {
	return (*unixSocket)(&u)
}

func (u *unixSocket) apply(opts *options) {
	if opts.hc == nil {
		opts.hc = &http.Client{}
	}

	opts.err = hcutil.UnixSocket(opts.hc, string(*u))
}
