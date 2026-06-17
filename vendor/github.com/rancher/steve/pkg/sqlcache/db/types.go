//go:generate go tool -modfile ../../../gotools/msgp/go.mod msgp -unexported -o types.msgp.go
package db

type unstructuredObject map[string]any
