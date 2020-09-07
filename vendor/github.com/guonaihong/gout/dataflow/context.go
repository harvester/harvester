package dataflow

// Context struct
type Context struct {
	Code  int //http code
	Error error

	*DataFlow
}
