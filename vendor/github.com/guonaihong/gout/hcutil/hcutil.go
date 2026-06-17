package hcutil

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/proxy"
)

func ModifyURL(url string) string {
	if strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://") {
		return url
	}

	if strings.HasPrefix(url, ":") {
		return fmt.Sprintf("http://127.0.0.1%s", url)
	}

	if strings.HasPrefix(url, "/") {
		return fmt.Sprintf("http://127.0.0.1%s", url)
	}

	return fmt.Sprintf("http://%s", url)
}

// socks5://username:password@localhost:7890
// socks5://localhost:7890
// localhost:7890
func SetSOCKS5(c *http.Client, socks5URL string) error {
	host := socks5URL
	var auth *proxy.Auth
	if strings.HasPrefix(socks5URL, "socks5://") {
		proxyURL, err := url.Parse(socks5URL)
		if err != nil {
			return err
		}

		if proxyURL.User != nil {
			password, _ := proxyURL.User.Password()
			auth = &proxy.Auth{
				User:     proxyURL.User.Username(),
				Password: password,
			}
		}

		host = proxyURL.Host
	}
	dialer, err := proxy.SOCKS5("tcp", host, auth, proxy.Direct)
	if err != nil {
		return err
	}

	if c.Transport == nil {
		c.Transport = &http.Transport{}
	}

	transport, ok := c.Transport.(*http.Transport)
	if !ok {
		return fmt.Errorf("SetSOCKS5:not found http.transport:%T", c.Transport)
	}

	transport.Dial = dialer.Dial
	return nil
}

func SetProxy(c *http.Client, proxyURL string) error {
	proxy, err := url.Parse(ModifyURL(proxyURL))
	if err != nil {
		return err
	}

	if c.Transport == nil {
		c.Transport = &http.Transport{}
	}

	transport, ok := c.Transport.(*http.Transport)
	if !ok {
		return fmt.Errorf("SetProxy:not found http.transport:%T", c.Transport)
	}

	transport.Proxy = http.ProxyURL(proxy)

	return nil
}

func UnixSocket(c *http.Client, path string) error {
	if c.Transport == nil {
		c.Transport = &http.Transport{}
	}

	transport, ok := c.Transport.(*http.Transport)
	if !ok {
		return fmt.Errorf("UnixSocket:not found http.transport:%T", c.Transport)
	}

	transport.DialContext = func(ctx context.Context, proto, addr string) (conn net.Conn, err error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", path)
	}

	return nil
}
