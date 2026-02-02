package internal

import (
	"fmt"
	"math/big"
)

type BigInt struct {
	big.Int
}

func (b BigInt) DeepCopyInto(n *BigInt) {
	n.FillBytes(b.Bytes())
}

func (b BigInt) Equal(n BigInt) bool {
	return b.Cmp(n) == 0
}

func (b BigInt) EqualInt64(n int64) bool {
	return b.Int.Cmp(big.NewInt(n)) == 0
}

func (b BigInt) Cmp(n BigInt) int {
	return b.Int.Cmp(&n.Int)
}

func (b BigInt) Add(n BigInt) BigInt {
	return BigInt{*big.NewInt(0).Add(&b.Int, &n.Int)}
}

func (b BigInt) Sub(n BigInt) BigInt {
	return BigInt{*big.NewInt(0).Sub(&b.Int, &n.Int)}
}

func (b BigInt) String() string {
	return b.Int.String()
}

func (b BigInt) MarshalJSON() ([]byte, error) {
	return []byte(b.String()), nil
}

func (b *BigInt) UnmarshalJSON(p []byte) error {
	if string(p) == "null" {
		return nil
	}
	var z big.Int
	_, ok := z.SetString(string(p), 10)
	if !ok {
		return fmt.Errorf("invalid big integer: %q", p)
	}
	b.Int = z
	return nil
}
