package decode

import (
	"net/http"
)

// Decoder Decoder interface
type Decoder2 interface {
	Decode(*http.Request, interface{})
}

var (
	// Header is the http header decoder
	Header = headerDecode{}
)
