// 参考 https://github.com/gin-gonic/gin/blob/master/internal/json/jsoniter.go

//go:build !jsoniter && !go_json && !(sonic && avx && (linux || windows || darwin) && amd64)
// +build !jsoniter
// +build !go_json
// +build !sonic !avx !linux,!windows,!darwin !amd64

package json

import "encoding/json"

var (
	Marshal    = json.Marshal
	Unmarshal  = json.Unmarshal
	NewDecoder = json.NewDecoder
	NewEncoder = json.NewEncoder
	Valid      = json.Valid
)
