package jobq

import (
	"context"
	"sync"
	"time"
)

// JobRunner interface is the accepted type for a job to be dispatched in the background.
type JobRunner interface {
	Run(ctx context.Context) (interface{}, error)
}

// JobRunnerFunc is a type used to wrap pure functions, allowing them to implement the JobRunner interface.
type JobRunnerFunc func(ctx context.Context) (interface{}, error)

// Run implements the JobRunner interface for a JobRunnerFunc.
func (f JobRunnerFunc) Run(ctx context.Context) (interface{}, error) {
	return f(ctx)
}

// JobTracker is the interface made available to control job or access the status
type JobTracker interface {
	ID() uint
	Done() <-chan struct{}
	Result() (interface{}, error)
	Status() JobStatus
	Stop()
}

// JobStatus represents the status of a job at a given instant in time.
type JobStatus struct {
	ID         uint
	Complete   bool
	Success    bool
	Error      string
	Payload    interface{}
	StartedAt  time.Time
	FinishedAt time.Time
}

type job struct {
	mu         sync.RWMutex
	id         uint
	complete   bool
	success    bool
	err        error
	payload    interface{}
	startedAt  time.Time
	finishedAt time.Time
	done       chan struct{}
	run        func()
	ctx        context.Context
	cancel     context.CancelFunc
	errC       chan error
	payloadC   chan interface{}
	metric     Metric
}

func (j *job) ID() uint {
	return j.id
}

func (j *job) Done() <-chan struct{} {
	return j.done
}

func (j *job) Result() (interface{}, error) {
	<-j.Done()
	return j.payload, j.err
}

func (j *job) Status() JobStatus {
	j.mu.RLock()
	defer j.mu.RUnlock()

	var errMsg string
	if j.err != nil {
		errMsg = j.err.Error()
	}

	return JobStatus{
		ID:         j.id,
		Complete:   j.complete,
		Success:    j.success,
		Error:      errMsg,
		Payload:    j.payload,
		StartedAt:  j.startedAt,
		FinishedAt: j.finishedAt,
	}
}

func (j *job) Stop() {
	j.cancel()
}

func (j *job) close(payload interface{}, err error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if j.complete {
		return
	}

	select {
	case <-j.ctx.Done():
		j.payload = nil
		j.err = j.ctx.Err()
	default:
		j.payload = payload
		j.err = err
	}

	j.finishedAt = time.Now()
	j.complete = true
	j.success = err == nil
	j.cancel()
	close(j.done)
}

func (j *job) listen() {
	select {
	case <-j.ctx.Done():
		j.close(nil, j.ctx.Err())
		if j.ctx.Err() == context.DeadlineExceeded {
			j.metric.IncJobTimeout()
		}

	case err := <-j.errC:
		j.close(nil, err)
	case payload := <-j.payloadC:
		j.close(payload, nil)
	}
}

func newJob(ctx context.Context, cancel context.CancelFunc, j JobRunner, id uint, metric Metric) *job {
	jb := &job{
		id:        id,
		startedAt: time.Now(),
		done:      make(chan struct{}),
		complete:  false,
		success:   false,
		ctx:       ctx,
		cancel:    cancel,
		errC:      make(chan error, 1),
		payloadC:  make(chan interface{}, 1),
		metric:    metric,
	}

	var once sync.Once
	jb.run = func() {
		once.Do(func() {
			go func() {
				payload, err := j.Run(jb.ctx)
				if err == nil {
					jb.payloadC <- payload
				} else {
					jb.errC <- err
				}
			}()
		})
	}

	go jb.listen()

	return jb
}
