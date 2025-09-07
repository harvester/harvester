package gout

import (
	"net/http"
	"time"

	"github.com/guonaihong/gout/dataflow"
	"github.com/guonaihong/gout/debug"
	_ "github.com/guonaihong/gout/export"
	_ "github.com/guonaihong/gout/filter"
)

// debug
type DebugOption = debug.Options //不推荐gout.DebugOption方式引用, 推荐debug.Options引用
type DebugOpt = debug.Apply      //不推荐gout.DebugOpt方式引用，推荐debug.Apply方式引用
type DebugFunc = debug.Func      //不推荐gout.DebugFunc方式引用，推荐debug.Func方式引用

func NoColor() DebugOpt {
	return debug.NoColor()
}

func Trace() DebugOpt {
	return debug.Trace()
}

type Context = dataflow.Context

// New function is mainly used when passing custom http client
func New(c ...*http.Client) *dataflow.Gout {
	return dataflow.New(c...)
}

// GET send HTTP GET method
// 第一种情况
// gout.GET("wwww.demo.xx/test-appkey")
//
// 第二种情况
//
//	type host struct {
//	 Host string
//	 AppKey string
//	}
//
// gout.GET("http://{{.Host}/{{.AppKey}}}", &host{Host:"www.demo.xx", AppKey:"test-appkey"})
func GET(url string, urlStruct ...interface{}) *dataflow.DataFlow {
	return dataflow.New().GET(url, urlStruct...)
}

// POST send HTTP POST method
func POST(url string, urlStruct ...interface{}) *dataflow.DataFlow {
	return dataflow.New().POST(url, urlStruct...)
}

// PUT send HTTP PUT method
func PUT(url string, urlStruct ...interface{}) *dataflow.DataFlow {
	return dataflow.New().PUT(url, urlStruct...)
}

// DELETE send HTTP DELETE method
func DELETE(url string, urlStruct ...interface{}) *dataflow.DataFlow {
	return dataflow.New().DELETE(url, urlStruct...)
}

// PATCH send HTTP PATCH method
func PATCH(url string, urlStruct ...interface{}) *dataflow.DataFlow {
	return dataflow.New().PATCH(url, urlStruct...)
}

// HEAD send HTTP HEAD method
func HEAD(url string, urlStruct ...interface{}) *dataflow.DataFlow {
	return dataflow.New().HEAD(url, urlStruct...)
}

// OPTIONS send HTTP OPTIONS method
func OPTIONS(url string, urlStruct ...interface{}) *dataflow.DataFlow {
	return dataflow.New().OPTIONS(url, urlStruct...)
}

// 设置不忽略空值
func NotIgnoreEmpty() {
	dataflow.GlobalSetting.NotIgnoreEmpty = true
}

// 设置忽略空值
func IgnoreEmpty() {
	dataflow.GlobalSetting.NotIgnoreEmpty = false
}

// 设置超时时间,
// d > 0, 设置timeout
// d == 0，取消全局变量
func SetTimeout(d time.Duration) {
	dataflow.GlobalSetting.SetTimeout(d)
}

func SetDebug(b bool) {
	dataflow.GlobalSetting.SetDebug(b)
}
