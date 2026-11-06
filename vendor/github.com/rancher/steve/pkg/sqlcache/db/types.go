//go:generate msgp -unexported -o types.msgp.go
package db

type unstructuredObject map[string]any
