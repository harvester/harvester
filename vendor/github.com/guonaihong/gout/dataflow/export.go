package dataflow

type export struct {
	df *DataFlow
}

func (e *export) Curl() Curl {
	filterMu.RLock()
	defer filterMu.RUnlock()

	filter, ok := filters["curl"]
	if !ok {
		panic("export.curl: not found")
	}

	b := filter.New(e.df)
	curl, ok := b.(Curl)
	if !ok {
		panic("export.curl not found Curl")
	}
	return curl
}
