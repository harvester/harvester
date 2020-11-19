package gout

import (
	"github.com/guonaihong/gout/dataflow"
	_ "github.com/guonaihong/gout/export"
	_ "github.com/guonaihong/gout/filter"
	"net/http"
)

// debug
type DebugOption = dataflow.DebugOption
type DebugOpt = dataflow.DebugOpt
type DebugFunc = dataflow.DebugFunc

func NoColor() DebugOpt {
	return dataflow.NoColor()
}

func Trace() DebugOpt {
	return dataflow.Trace()
}

type Context = dataflow.Context

// New function is mainly used when passing custom http client
func New(c ...*http.Client) *dataflow.Gout {
	return dataflow.New(c...)
}

// GET send HTTP GET method
func GET(url string) *dataflow.DataFlow {
	return dataflow.GET(url)
}

// POST send HTTP POST method
func POST(url string) *dataflow.DataFlow {
	return dataflow.POST(url)
}

// PUT send HTTP PUT method
func PUT(url string) *dataflow.DataFlow {
	return dataflow.PUT(url)
}

// DELETE send HTTP DELETE method
func DELETE(url string) *dataflow.DataFlow {
	return dataflow.DELETE(url)
}

// PATCH send HTTP PATCH method
func PATCH(url string) *dataflow.DataFlow {
	return dataflow.PATCH(url)
}

// HEAD send HTTP HEAD method
func HEAD(url string) *dataflow.DataFlow {
	return dataflow.HEAD(url)
}

// OPTIONS send HTTP OPTIONS method
func OPTIONS(url string) *dataflow.DataFlow {
	return dataflow.OPTIONS(url)
}
