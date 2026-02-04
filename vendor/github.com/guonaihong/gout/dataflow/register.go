package dataflow

import (
	"sync"
)

// 所有的filter通过调用Register函数注册到filters变量里面
// 这么做的目的是为了剪去DataFlow和filter之前的循环引用
// 使用注册机制好处如下
// DataFlow层(比如gout.GET()这种用法)可以通过.Bench().Retry调用filter层函数
// Bench层也可以使用DataFlow层功能

var (
	filterMu sync.RWMutex
	filters  = make(map[string]NewFilter)
)

func Register(name string, filter NewFilter) {
	filterMu.Lock()
	defer filterMu.Unlock()

	if filters == nil {
		panic("dataflow: Register filters is nil")
	}

	if _, dup := filters[name]; dup {
		panic("dataflow: Register called twice for driver " + name)
	}

	filters[name] = filter
}
