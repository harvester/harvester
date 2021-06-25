package jobq

import (
	"sync/atomic"
)

// TrackParams is the params input into report function.
type TrackParams struct {
	JobQueueSize uint64
	TotalWorkers uint64
	BusyWorkers  uint64
}

// Metric represents the contract that it must report corresponding metrics.
type Metric interface {
	SetTotalWorkers(v int)
	TotalWorkers() uint64
	SetCurrentJobQueueSize(v int)
	IncBusyWorker()
	DecBusyWorker()
	IncJobTimeout()
	IncJobsCounter()
	Report()
	JobTimeoutRate() float64
	WorkerLoading() float64
	ResetCounters()
}

type emptyMetric struct{}

func newEmptyMetric() Metric {
	return &emptyMetric{}
}

func (e *emptyMetric) SetTotalWorkers(v int)        {}
func (e *emptyMetric) TotalWorkers() uint64         { return 0 }
func (e *emptyMetric) SetCurrentJobQueueSize(v int) {}
func (e *emptyMetric) IncBusyWorker()               {}
func (e *emptyMetric) DecBusyWorker()               {}
func (e *emptyMetric) IncJobTimeout()               {}
func (e *emptyMetric) IncJobsCounter()              {}
func (e *emptyMetric) Report()                      {}
func (e *emptyMetric) JobTimeoutRate() float64      { return 0.0 }
func (e *emptyMetric) WorkerLoading() float64       { return 0.0 }
func (e *emptyMetric) ResetCounters()               {}

type metric struct {
	reportFunc func(TrackParams)

	// for prometheus metrics usage
	curJobQueueSize uint64
	totalWorkers    uint64
	busyWorkers     uint64

	// for calculating adjust workers
	jobTimeouts uint64
	jobsCounter uint64
}

func newMetric(reportFunc func(TrackParams)) Metric {
	return &metric{
		reportFunc: reportFunc,
	}
}

func (m *metric) Report() {
	m.reportFunc(
		TrackParams{
			TotalWorkers: atomic.LoadUint64(&m.totalWorkers),
			BusyWorkers:  atomic.LoadUint64(&m.busyWorkers),
			JobQueueSize: atomic.LoadUint64(&m.curJobQueueSize),
		},
	)
}

func (m *metric) TotalWorkers() uint64 {
	return atomic.LoadUint64(&m.totalWorkers)
}

func (m *metric) JobTimeoutRate() float64 {
	return (float64)(atomic.LoadUint64(&m.jobTimeouts)) / (float64)(atomic.LoadUint64(&m.jobsCounter))
}

func (m *metric) WorkerLoading() float64 {
	return (float64)(atomic.LoadUint64(&m.busyWorkers)) / (float64)(atomic.LoadUint64(&m.totalWorkers))
}

func (m *metric) ResetCounters() {
	atomic.StoreUint64(&m.jobTimeouts, 0)
	atomic.StoreUint64(&m.jobsCounter, 0)
}

func (m *metric) IncJobsCounter() {
	atomic.AddUint64(&m.jobsCounter, 1)
}

func (m *metric) IncJobTimeout() {
	atomic.AddUint64(&m.jobTimeouts, 1)
}

func (m *metric) SetTotalWorkers(v int) {
	if v >= 0 {
		atomic.StoreUint64(&m.totalWorkers, uint64(v))
	}
}

func (m *metric) SetCurrentJobQueueSize(v int) {
	if v >= 0 {
		atomic.StoreUint64(&m.curJobQueueSize, uint64(v))
	}
}

func (m *metric) IncBusyWorker() {
	atomic.AddUint64(&m.busyWorkers, 1)
}

func (m *metric) DecBusyWorker() {
	atomic.AddUint64(&m.busyWorkers, ^uint64(0))
}
