package kubernetes

type temporaryError struct {
	error
}

func (t *temporaryError) Temporary() bool {
	return true
}
