package filter

import (
	"context"
	"errors"
	"fmt"
	"github.com/guonaihong/gout/dataflow"
	"math"
	"math/rand"
	"net/http"
	"time"
)

var (
	// RetryWaitTime retry basic wait time
	RetryWaitTime = 200 * time.Millisecond
	// RetryMaxWaitTime Maximum retry wait time
	RetryMaxWaitTime = 10 * time.Second
	// RetryAttempt number of retries
	RetryAttempt = 1
)

var (
	ErrRetryFail = errors.New("retry fail")
	ErrRetry     = errors.New("need to retry")
)

// Retry is the core data structure of the retry function
// https://amazonaws-china.com/cn/blogs/architecture/exponential-backoff-and-jitter/
type Retry struct {
	df          *dataflow.DataFlow
	attempt     int // Maximum number of attempts
	currAttempt int
	maxWaitTime time.Duration
	waitTime    time.Duration
	cb          func(c *dataflow.Context) error
}

func (r *Retry) New(df *dataflow.DataFlow) interface{} {
	return &Retry{df: df}
}

// Attempt set the number of retries
func (r *Retry) Attempt(attempt int) dataflow.Retry {
	r.attempt = attempt
	return r
}

// WaitTime sets the basic wait time
func (r *Retry) WaitTime(waitTime time.Duration) dataflow.Retry {
	r.waitTime = waitTime
	return r
}

// MaxWaitTime Sets the maximum wait time
func (r *Retry) MaxWaitTime(maxWaitTime time.Duration) dataflow.Retry {
	r.maxWaitTime = maxWaitTime
	return r
}

func (r *Retry) Func(cb func(c *dataflow.Context) error) dataflow.Retry {
	r.cb = cb
	return r
}

func (r *Retry) reset() {
	r.currAttempt = 0
}

func (r *Retry) init() {
	if r.attempt == 0 {
		r.attempt = RetryAttempt
	}

	if r.waitTime == 0 {
		r.waitTime = RetryWaitTime
	}

	if r.maxWaitTime == 0 {
		r.maxWaitTime = RetryMaxWaitTime
	}
}

// Does not pollute the namespace
func (r *Retry) min(a, b uint64) uint64 {
	if a > b {
		return b
	}
	return a
}

func (r *Retry) getSleep() time.Duration {
	temp := uint64(r.waitTime * time.Duration(math.Exp2(float64(r.currAttempt))))
	if temp <= 0 {
		temp = uint64(r.waitTime)
	}
	temp = r.min(uint64(r.maxWaitTime), uint64(temp))
	//对int64边界处理, 后面使用rand.Int63n所以,最大值只能是int64的最大值防止溢出
	if temp > math.MaxInt64 {
		temp = math.MaxInt64
	}

	temp /= 2
	return time.Duration(temp) + time.Duration(rand.Int63n(int64(temp)))
}

func (r *Retry) genContext(resp *http.Response, err error) *dataflow.Context {
	code := 0
	if resp != nil {
		code = resp.StatusCode
	}
	return &dataflow.Context{DataFlow: r.df, Error: err, Code: code}
}

// Do send function
func (r *Retry) Do() (err error) {
	defer r.reset()
	r.init()

	req, err := r.df.Request()
	if err != nil {
		return err
	}

	tk := time.NewTimer(r.maxWaitTime)
	client := r.df.Client()

	for i := 0; i < r.attempt; i++ {

		// 这里只要调用Func方法，且回调函数返回ErrRetry 会生成新的*http.Request对象
		// 不使用DataFlow.Do()方法原因基于两方面考虑
		// 1.为了效率只需经过一次编码器得到*http.Request,如果需要重试几次后面是多次使用解码器.Bind()函数
		// 2.为了更灵活的控制
		resp, err := client.Do(req)
		if r.cb != nil {
			err = r.cb(r.genContext(resp, err))
			if err != nil {
				if resp != nil {
					r.df.Bind(req, resp) //为的是输出debug信息
					resp.Body.Close()
				}

				if err != ErrRetry {
					return err
				}

				var err2 error
				req, err2 = r.df.Request()
				if err2 != nil {
					return err2
				}
			}
		}

		if err == nil && resp != nil {
			defer resp.Body.Close()
			return r.df.Bind(req, resp)
		}

		sleep := r.getSleep()

		if r.df.IsDebug() {
			fmt.Printf("filter:retry #current attempt:%d, wait time %v\n", r.currAttempt, sleep)
		}

		tk.Reset(sleep)
		ctx := r.df.GetContext()
		if ctx == nil {
			ctx = context.Background()
		}

		select {
		case <-tk.C:
			// 外部可以使用context直接取消
		case <-ctx.Done():
			return ctx.Err()
		}

		r.currAttempt++
	}

	return ErrRetryFail
}
