package jobq

const (
	defaultJobTimeoutRateUpperBoundPercentage float64 = 80.0
	defaultJobTimeoutRateLowerBoundPercentage float64 = 20.0
	defaultWorkerLoadingUpperBoundPercentage  float64 = 80.0
	defaultWorkerLoadingLowerBoundPercentage  float64 = 20.0
	defaultWorkersNumUpperBound               int     = 2 << 17
	defaultWorkersNumLowerBound               int     = 2 << 9
	defaultWorkerMargin                       float64 = 2.0
	defaultWorkerPoolSize                             = 2 << 9
)

// scaler calculates how many worker should be enabled.
type scaler struct {
	jobTimeoutRateUpperBoundPercentage float64
	jobTimeoutRateLowerBoundPercentage float64
	workerLoadingUpperBoundPercentage  float64
	workerLoadingLowerBoundPercentage  float64
	workersNumUpperBound               int
	workersNumLowerBound               int
	workerMargin                       float64
	workerPoolSize                     int
}

func newScaler() *scaler {
	return &scaler{
		jobTimeoutRateUpperBoundPercentage: defaultJobTimeoutRateUpperBoundPercentage,
		jobTimeoutRateLowerBoundPercentage: defaultJobTimeoutRateLowerBoundPercentage,
		workerLoadingUpperBoundPercentage:  defaultWorkerLoadingUpperBoundPercentage,
		workerLoadingLowerBoundPercentage:  defaultWorkerLoadingLowerBoundPercentage,
		workersNumUpperBound:               defaultWorkersNumUpperBound,
		workersNumLowerBound:               defaultWorkersNumLowerBound,
		workerMargin:                       defaultWorkerMargin,
		workerPoolSize:                     defaultWorkerPoolSize,
	}
}

type vote int

const (
	voteNoChange vote = iota
	voteIncrease
	voteDecrease
)

func (s *scaler) scale(m Metric) int {
	loading := m.WorkerLoading() * 100
	timeoutRate := m.JobTimeoutRate() * 100
	totalWorkers := float64(m.TotalWorkers())

	checks := []func() vote{
		func() vote {
			if loading >= s.workerLoadingUpperBoundPercentage {
				return voteIncrease
			} else if loading <= s.workerLoadingLowerBoundPercentage {
				return voteDecrease
			}
			return voteNoChange
		},
		func() vote {
			if timeoutRate >= s.jobTimeoutRateUpperBoundPercentage {
				return voteIncrease
			} else if timeoutRate <= s.jobTimeoutRateLowerBoundPercentage {
				return voteDecrease
			}
			return voteNoChange
		},
	}

	var increase, decrease, nochange uint
	for _, check := range checks {
		switch check() {
		case voteDecrease:
			decrease++
		case voteIncrease:
			increase++
		case voteNoChange:
			nochange++
		}
	}

	result := voteNoChange
	if increase > 0 {
		result = voteIncrease
	} else if decrease > nochange {
		result = voteDecrease
	}

	switch result {
	case voteIncrease:
		totalWorkers *= s.workerMargin
	case voteDecrease:
		totalWorkers /= s.workerMargin
	}

	workerNum := int(totalWorkers)
	if workerNum < s.workersNumLowerBound {
		workerNum = s.workersNumLowerBound
	} else if workerNum > s.workersNumUpperBound {
		workerNum = s.workersNumUpperBound
	}

	m.ResetCounters()

	return workerNum
}
