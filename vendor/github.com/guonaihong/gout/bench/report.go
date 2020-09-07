package bench

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

var _ SubTasker = (*Report)(nil)

type result struct {
	time       time.Duration
	statusCode int
}

// 数据字段，每个字段都用于显示
type report struct {
	Concurrency     int    //并发数
	Failed          uint64 //出错的连接数
	CompleteRequest uint64 //正常的请求数
	TotalRead       uint64 //统计所有read的流量body+(request line)+(http header)
	TotalBody       uint64 //统计所有body的流量
	TotalWriteBody  uint64 //统计所有写入的body流量
	Tps             float64
	Duration        time.Duration // 连接总时间
	Kbs             float64
	Mean            float64
	AllMean         float64
	Percentage55    time.Duration
	Percentage66    time.Duration
	Percentage75    time.Duration
	Percentage80    time.Duration
	Percentage90    time.Duration
	Percentage95    time.Duration
	Percentage98    time.Duration
	Percentage99    time.Duration
	Percentage100   time.Duration
	StatusCodes     map[int]int
	ErrMsg          map[string]int
}

// Report 是报表核心数据结构
type Report struct {
	SendNum int // 已经发送的http 请求
	report
	Number     int // 发送总次数
	step       int // 动态报表输出间隔
	allResult  chan result
	waitQuit   chan struct{} //等待startReport函数结束
	allTimes   []time.Duration
	ctx        context.Context
	cancel     func()
	getRequest func() (*http.Request, error)

	startTime time.Time
	*http.Client

	lerr  sync.Mutex
	lcode sync.Mutex
}

// NewReport is a report initialization function
func NewReport(ctx context.Context,
	c, n int,

	duration time.Duration,

	getRequest func() (*http.Request, error),

	client *http.Client) *Report {
	step := 0
	if n > 150 {
		if step = n / 10; step < 100 {
			step = 10
		}
	}

	ctx, cancel := context.WithCancel(ctx)
	return &Report{
		allResult: make(chan result),
		report: report{
			Concurrency: c,
			StatusCodes: make(map[int]int, 1000),
			Duration:    duration,
			ErrMsg:      make(map[string]int, 2),
		},
		waitQuit:   make(chan struct{}),
		Number:     n,
		step:       step,
		ctx:        ctx,
		cancel:     cancel,
		getRequest: getRequest,
		Client:     client,
		startTime:  time.Now(),
	}
}

// Cancel report logic
func (r *Report) Cancel() {
	r.cancel()
}

// Init 初始化报表模块, 后台会起一个统计go程
func (r *Report) Init() {
	r.startReport()
}

func (r *Report) addComplete() {
	atomic.AddUint64(&r.CompleteRequest, 1)
}

// 统计错误消息
func (r *Report) addErrAndFailed(err error) {
	r.lerr.Lock()
	r.ErrMsg[err.Error()]++
	r.lerr.Unlock()
	atomic.AddUint64(&r.Failed, 1)
}

// 统计http code数量
func (r *Report) addCode(code int) {
	r.lcode.Lock()
	r.StatusCodes[code]++
	r.lcode.Unlock()
}

// Process 负责构造压测http 链接和统计压测元数据
func (r *Report) Process(work chan struct{}) {
	for range work {
		start := time.Now()

		req, err := r.getRequest()
		if err != nil {
			r.addErrAndFailed(err)
			continue
		}

		resp, err := r.Do(req)
		if err != nil {
			r.addErrAndFailed(err)
			continue
		}

		body, _ := req.GetBody()
		if body != nil {
			bodySize, _ := io.Copy(ioutil.Discard, body)
			atomic.AddUint64(&r.TotalWriteBody, uint64(bodySize))
		}

		// 统计http code数量
		r.addCode(resp.StatusCode)

		bodySize, err := io.Copy(ioutil.Discard, resp.Body)

		r.calBody(resp, uint64(bodySize))

		resp.Body.Close()

		r.addComplete()
		r.allResult <- result{
			time:       time.Now().Sub(start),
			statusCode: resp.StatusCode,
		}
	}
}

// WaitAll 等待结束
func (r *Report) WaitAll() {
	<-r.waitQuit
	r.outputReport() //输出最终报表
}

func (r *Report) calBody(resp *http.Response, bodySize uint64) {

	hN := len(resp.Status)
	hN += len(resp.Proto)
	hN++    //space
	hN += 2 //\r\n
	for k, v := range resp.Header {
		hN += len(k)

		for _, hv := range v {
			hN += len(hv)
		}
		hN += 2 //:space
		hN += 2 //\r\n
	}

	hN += 2

	atomic.AddUint64(&r.TotalBody, uint64(bodySize))
	atomic.AddUint64(&r.TotalRead, uint64(hN))
	atomic.AddUint64(&r.TotalRead, uint64(bodySize))

}

func genTimeStr(now time.Time) string {
	year, month, day := now.Date()
	hour, min, sec := now.Clock()

	return fmt.Sprintf("%4d-%02d-%02d %02d:%02d:%02d.%06d",
		year,
		month,
		day,
		hour,
		min,
		sec,
		now.Nanosecond()/1e3,
	)
}

func (r *Report) startReport() {
	go func() {
		defer func() {
			fmt.Printf("  Finished  %15d requests\n", r.SendNum)
			r.waitQuit <- struct{}{}
		}()

		if r.step > 0 {
			for {
				select {
				case <-r.ctx.Done():
					return
				case v := <-r.allResult:
					r.SendNum++
					if r.step > 0 && r.SendNum%r.step == 0 {
						now := time.Now()

						fmt.Printf("    Opened %15d connections: [%s]\n",
							r.SendNum, genTimeStr(now))
					}

					r.allTimes = append(r.allTimes, v.time)
				}
			}
		}

		begin := time.Now()
		interval := r.Duration / 10

		if interval == 0 || int64(interval) > int64(3*time.Second) {
			interval = 3 * time.Second
		}

		nTick := time.NewTicker(interval)
		count := 1
		for {
			select {
			case <-nTick.C:
				now := time.Now()

				fmt.Printf("  Completed %15d requests [%s]\n",
					r.SendNum, genTimeStr(now))

				count++
				next := begin.Add(time.Duration(count * int(interval)))
				if newInterval := next.Sub(time.Now()); newInterval > 0 {
					nTick = time.NewTicker(newInterval)
				} else {
					nTick = time.NewTicker(time.Millisecond * 100)
				}
			case v, ok := <-r.allResult:
				if !ok {
					return
				}

				r.SendNum++
				r.allTimes = append(r.allTimes, v.time)
			case <-r.ctx.Done():
				return
			}
		}

	}()
}

func (r *Report) outputReport() {
	r.Duration = time.Now().Sub(r.startTime)
	r.Tps = float64(r.SendNum) / r.Duration.Seconds()
	r.AllMean = float64(r.Concurrency) * float64(r.Duration) / float64(time.Millisecond) / float64(r.SendNum)
	r.Mean = float64(r.Duration) / float64(r.SendNum) / float64(time.Millisecond)
	r.Kbs = float64(r.TotalRead) / float64(1024) / r.Duration.Seconds()

	allTimes := r.allTimes
	sort.Slice(allTimes, func(i, j int) bool {
		return allTimes[i] < allTimes[j]
	})

	if len(allTimes) > 1 {
		r.Percentage55 = allTimes[int(float64(len(allTimes))*0.5)]
		r.Percentage66 = allTimes[int(float64(len(allTimes))*0.66)]
		r.Percentage75 = allTimes[int(float64(len(allTimes))*0.75)]
		r.Percentage80 = allTimes[int(float64(len(allTimes))*0.80)]
		r.Percentage90 = allTimes[int(float64(len(allTimes))*0.90)]
		r.Percentage95 = allTimes[int(float64(len(allTimes))*0.95)]
		r.Percentage98 = allTimes[int(float64(len(allTimes))*0.98)]
		r.Percentage99 = allTimes[int(float64(len(allTimes))*0.99)]
		r.Percentage100 = allTimes[len(allTimes)-1]
	}

	tmpl := newTemplate()
	tmpl.Execute(os.Stdout, r.report)
}
