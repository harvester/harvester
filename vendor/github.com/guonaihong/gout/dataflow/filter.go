package dataflow

type filter struct {
	df *DataFlow
}

func (f *filter) Bench() Bencher {
	filterMu.RLock()
	defer filterMu.RUnlock()

	filter, ok := filters["bench"]
	if !ok {
		panic("filter.bench: not found")
	}

	b := filter.New(f.df)
	bench, ok := b.(Bencher)
	if !ok {
		panic("filter.bench not found Bencher interface")
	}
	return bench

}

func (f *filter) Retry() Retry {
	filterMu.RLock()
	defer filterMu.RUnlock()

	filter, ok := filters["retry"]
	if !ok {
		panic("filter.retry: not found interface")
	}

	b := filter.New(f.df)
	retry, ok := b.(Retry)
	if !ok {
		panic("filter.retry not found Retry")
	}
	return retry
}
