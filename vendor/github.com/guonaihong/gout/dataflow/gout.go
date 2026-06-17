package dataflow

import (
	"net/http"
)

// Gout is the data structure at the beginning of everything
type Gout struct {
	*http.Client
	DataFlow // TODO 优化
}

var (
	// DefaultClient The default http client, which has a connection pool
	DefaultClient = http.Client{}
	// DefaultBenchClient is the default http client used by the benchmark,
	// which has a connection pool
	DefaultBenchClient = http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 10000,
		},
	}
)

// New function is mainly used when passing custom http client
func New(c ...*http.Client) *Gout {
	out := &Gout{}
	if len(c) == 0 || c[0] == nil {
		out.Client = &DefaultClient
	} else {
		out.Client = c[0]
	}

	out.DataFlow.out = out
	out.DataFlow.Req.g = out
	return out
}

// TODO 这一层可以直接删除
// v0.3.3版本开始算起， v0.3.7版本将会删除
// GET send HTTP GET method
func GET(url string) *DataFlow {
	return New().GET(url)
}

// POST send HTTP POST method
// v0.3.3版本开始算起， v0.3.7版本将会删除
func POST(url string) *DataFlow {
	return New().POST(url)
}

// PUT send HTTP PUT method
// v0.3.3版本开始算起， v0.3.7版本将会删除
func PUT(url string) *DataFlow {
	return New().PUT(url)
}

// DELETE send HTTP DELETE method
// v0.3.3版本开始算起， v0.3.7版本将会删除
func DELETE(url string) *DataFlow {
	return New().DELETE(url)
}

// PATCH send HTTP PATCH method
// v0.3.3版本开始算起， v0.3.7版本将会删除
func PATCH(url string) *DataFlow {
	return New().PATCH(url)
}

// HEAD send HTTP HEAD method
// v0.3.3版本开始算起， v0.3.7版本将会删除
func HEAD(url string) *DataFlow {
	return New().HEAD(url)
}

// OPTIONS send HTTP OPTIONS method
// v0.3.3版本开始算起， v0.3.7版本将会删除
func OPTIONS(url string) *DataFlow {
	return New().OPTIONS(url)
}
