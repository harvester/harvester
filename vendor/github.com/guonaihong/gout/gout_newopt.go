package gout

import (
	"net/http"

	"github.com/guonaihong/gout/dataflow"
)

type Client struct {
	options
}

// NewWithOpt 设计哲学
// 1.一些不经常变化的配置放到NewWithOpt里面实现
// 2.一些和http.Client深度绑定的放到NewWithOpt里面实现
// 3.一些可以提升使用体验的放到NewWithOpt里面实现
func NewWithOpt(opts ...Option) *Client {
	c := &Client{}
	c.hc = &http.Client{}

	for _, o := range opts {
		o.apply(&c.options)
	}

	return c

}

// GET send HTTP GET method
func (c *Client) GET(url string) *dataflow.DataFlow {
	return dataflow.New(c.hc).GET(url).SetSetting(c.Setting)
}

// POST send HTTP POST method
func (c *Client) POST(url string) *dataflow.DataFlow {
	return dataflow.New(c.hc).POST(url).SetSetting(c.Setting)
}

// PUT send HTTP PUT method
func (c *Client) PUT(url string) *dataflow.DataFlow {
	return dataflow.New(c.hc).PUT(url).SetSetting(c.Setting)
}

// DELETE send HTTP DELETE method
func (c *Client) DELETE(url string) *dataflow.DataFlow {
	return dataflow.New(c.hc).DELETE(url).SetSetting(c.Setting)
}

// PATCH send HTTP PATCH method
func (c *Client) PATCH(url string) *dataflow.DataFlow {
	return dataflow.New(c.hc).PATCH(url).SetSetting(c.Setting)
}

// HEAD send HTTP HEAD method
func (c *Client) HEAD(url string) *dataflow.DataFlow {
	return dataflow.New(c.hc).HEAD(url).SetSetting(c.Setting)
}

// OPTIONS send HTTP OPTIONS method
func (c *Client) OPTIONS(url string) *dataflow.DataFlow {
	return dataflow.New(c.hc).OPTIONS(url).SetSetting(c.Setting)
}
