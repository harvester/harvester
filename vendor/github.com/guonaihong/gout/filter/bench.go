package filter

import (
	"context"
	"github.com/guonaihong/gout/bench"
	"github.com/guonaihong/gout/core"
	"github.com/guonaihong/gout/dataflow"
	"net/http"
	"time"
)

// Bench provide benchmark features
type Bench struct {
	bench.Task

	df *dataflow.DataFlow

	r          *bench.Report
	getRequest func() (*http.Request, error)
}

func NewBench() *Bench {
	return &Bench{}
}

// New
func (b *Bench) New(df *dataflow.DataFlow) interface{} {
	return &Bench{df: df}
}

// Concurrent set the number of benchmarks for concurrency
func (b *Bench) Concurrent(c int) dataflow.Bencher {
	b.Task.Concurrent = c
	return b
}

// Number set the number of benchmarks
func (b *Bench) Number(n int) dataflow.Bencher {
	b.Task.Number = n
	return b
}

// Rate set the frequency of the benchmark
func (b *Bench) Rate(rate int) dataflow.Bencher {
	b.Task.Rate = rate
	return b
}

// Durations set the benchmark time
func (b *Bench) Durations(d time.Duration) dataflow.Bencher {
	b.Task.Duration = d
	return b
}

func (b *Bench) Loop(cb func(c *dataflow.Context) error) dataflow.Bencher {
	b.getRequest = func() (*http.Request, error) {
		c := dataflow.Context{}
		// TODO 优化，这里创建了两个dataflow对象
		// c.SetGout和 c.getDataFlow 都依赖gout对象
		// 后面的版本要先梳理下gout对象的定位
		out := dataflow.New()
		c.DataFlow = &dataflow.DataFlow{}
		c.SetGout(out)
		err := cb(&c)
		if err != nil {
			return nil, err
		}

		return c.Request()
	}

	return b
}

func (b *Bench) GetReport(r *bench.Report) dataflow.Bencher {
	b.r = r
	return b
}

// Do benchmark startup function
func (b *Bench) Do() error {
	// 报表插件
	if b.getRequest == nil {
		req, err := b.df.Req.Request()
		if err != nil {
			return err
		}

		b.getRequest = func() (*http.Request, error) {
			return core.CloneRequest(req)
		}
	}

	var client *http.Client
	if b.df != nil {
		client = b.df.Client()
	}

	if client == &dataflow.DefaultClient || client == nil {
		client = &dataflow.DefaultBenchClient
	}

	r := bench.NewReport(context.Background(),
		b.Task.Concurrent,
		b.Task.Number,
		b.Task.Duration,
		b.getRequest,
		client)

	// task是并发控制模块
	b.Run(r)

	if b.r != nil {
		*b.r = *r
	}
	return nil
}
