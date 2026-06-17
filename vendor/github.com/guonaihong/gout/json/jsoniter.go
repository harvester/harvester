//go:build jsoniter
// +build jsoniter

package json

import jsoniter "github.com/json-iterator/go"

var (
	json       = jsoniter.ConfigCompatibleWithStandardLibrary
	Marshal    = json.Marshal
	Unmarshal  = json.Unmarshal
	NewDecoder = json.NewDecoder
	NewEncoder = json.NewEncoder
	Valid      = json.Valid
)
