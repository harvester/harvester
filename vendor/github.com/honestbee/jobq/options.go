package jobq

import (
	"errors"
	"time"
)

// WorkerDispatcherOption provides a function way to
// configure a new WorkerDispatcher.
type WorkerDispatcherOption func(*WorkerDispatcher)

// WorkerN sets the number of workers to spawn.
func WorkerN(numWorkers int) WorkerDispatcherOption {
	if numWorkers <= 0 {
		panic(errors.New("the number of workers must greater than 0"))
	}
	return func(d *WorkerDispatcher) {
		d.scaler.workersNumLowerBound = numWorkers
	}
}

// WorkerPoolSize sets the worker pool size.
func WorkerPoolSize(size int) WorkerDispatcherOption {
	if size <= 0 {
		panic(errors.New("the worker pool size must greater than 0"))
	}
	return func(d *WorkerDispatcher) {
		d.scaler.workerPoolSize = size
	}
}

// EnableTrackReport enables the tracking report function,
// the input func sets how to report metrics.
// ***By turning this on (with non-nil input func), the below options go functional***
// 1. MetricsReportPeriod
func EnableTrackReport(f func(TrackParams)) WorkerDispatcherOption {
	if f == nil {
		panic(errors.New("the track report function should not be nil"))
	}
	return func(d *WorkerDispatcher) {
		d.reportFunc = f
	}
}

// MetricsReportPeriod sets the period of reporting prometheus metrics.
func MetricsReportPeriod(period time.Duration) WorkerDispatcherOption {
	if period <= 0 {
		panic(errors.New("the period of metrics report must greater than 0"))
	}
	return func(d *WorkerDispatcher) {
		d.metricsReportPeriod = period
	}
}

// EnableDynamicAdjustWorkers enables the dynamic adjusting worker mechanism,
// ***By turning this on, the following options go functional:***
// 1. WorkerAdjustPeriod
// 2. JobTimeoutRateBoundPercentage
// 3. WorkerLoadingBoundPercentage
// 4. WorkerMargin
func EnableDynamicAdjustWorkers(enable bool) WorkerDispatcherOption {
	return func(d *WorkerDispatcher) {
		d.enableDynamicWorker = enable
	}
}

// WorkerAdjustPeriod sets the period of checking
// 1. the timeout rate of jobs (timeout jobs / total jobs in a period)
// 2. the workers loading (busy workers / total workers)
// then adjust the number of workers properly.
func WorkerAdjustPeriod(period time.Duration) WorkerDispatcherOption {
	if period <= 0 {
		panic(errors.New("the period of adjusting workers must greater than 0"))
	}
	return func(d *WorkerDispatcher) {
		d.workerAdjusterPeriod = period
	}
}

// JobTimeoutRateBoundPercentage sets the job timeout upper and lower bounds of percentage to trigger increase or decrease the workers.
func JobTimeoutRateBoundPercentage(lower, upper float64) WorkerDispatcherOption {
	if upper <= 0.0 || 100.0 < upper {
		panic(errors.New("the upper bound of job timeout rate must be in (0, 100]"))
	}
	if lower <= 0.0 || 100.0 < lower {
		panic(errors.New("the lower bound of job timeout rate must be in (0, 100]"))
	}
	if upper <= lower {
		panic(errors.New("the job timeout rate of upper bound must greater than lower bound"))
	}
	return func(d *WorkerDispatcher) {
		d.scaler.jobTimeoutRateUpperBoundPercentage = upper
		d.scaler.jobTimeoutRateLowerBoundPercentage = lower
	}
}

// WorkerLoadingBoundPercentage sets the worker loading upper and lower bounds to trigger increase or decrease the workers.
func WorkerLoadingBoundPercentage(lower, upper float64) WorkerDispatcherOption {
	if upper <= 0.0 || 100.0 < upper {
		panic(errors.New("the upper bound of worker loading must be in (0, 100]"))
	}
	if lower <= 0.0 || 100.0 < lower {
		panic(errors.New("the lower bound of worker loading must be in (0, 100]"))
	}
	if upper <= lower {
		panic(errors.New("the worker loading of upper bound must greater than lower bound"))
	}
	return func(d *WorkerDispatcher) {
		d.scaler.workerLoadingUpperBoundPercentage = upper
		d.scaler.workerLoadingLowerBoundPercentage = lower
	}
}

// WorkerMargin sets the margin when increasing or decreasing workers.
func WorkerMargin(margin float64) WorkerDispatcherOption {
	if margin <= 0.0 {
		panic(errors.New("the margin must greater than 0"))
	}
	return func(d *WorkerDispatcher) {
		d.scaler.workerMargin = margin
	}
}
