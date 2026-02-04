package decode

import (
	"io"
)

// Decoder is the decoding interface
type Decoder interface {
	Decode(r io.Reader) error
}
