//go:build go_json
// +build go_json

package json

import json "github.com/goccy/go-json"

var (
	Marshal    = json.Marshal
	Unmarshal  = json.Unmarshal
	NewDecoder = json.NewDecoder
	NewEncoder = json.NewEncoder
	Valid      = json.Valid
)
