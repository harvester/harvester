package util

import (
	"fmt"
	"sync"

	"github.com/RoaringBitmap/roaring"
)

type Bitmap struct {
	base int32
	size int32
	data *roaring.Bitmap
	lock *sync.Mutex
}

// NewBitmap allocate a bitmap range from [start, end], notice the end is included
func NewBitmap(start, end int32) *Bitmap {
	size := end - start + 1
	data := roaring.New()
	if size > 0 {
		data.AddRange(0, uint64(size))
	}
	return &Bitmap{
		base: start,
		size: size,
		data: data,
		lock: &sync.Mutex{},
	}
}

func (b *Bitmap) AllocateRange(count int32) (int32, int32, error) {
	b.lock.Lock()
	defer b.lock.Unlock()

	if count <= 0 {
		return 0, 0, fmt.Errorf("invalid request for non-positive counts: %v", count)
	}
	i := b.data.Iterator()
	bStart := int32(0)
	for bStart <= b.size {
		last := int32(-1)
		remains := count
		for i.HasNext() && remains > 0 {
			// first element
			if last < 0 {
				last = int32(i.Next())
				bStart = last
				remains--
				continue
			}
			next := int32(i.Next())
			// failed to find the available range
			if next-last > 1 {
				break
			}
			last = next
			remains--
		}
		if remains == 0 {
			break
		}
		if !i.HasNext() {
			return 0, 0, fmt.Errorf("cannot find an empty port range")
		}
	}
	bEnd := bStart + count - 1
	b.data.RemoveRange(uint64(bStart), uint64(bEnd)+1)
	return b.base + bStart, b.base + bEnd, nil
}

func (b *Bitmap) ReleaseRange(start, end int32) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	if start == end && end == 0 {
		return nil
	}
	bStart := start - b.base
	bEnd := end - b.base
	if bStart < 0 || bEnd >= b.size {
		return fmt.Errorf("exceed range: %v-%v (%v-%v)", start, end, bStart, bEnd)
	}
	b.data.AddRange(uint64(bStart), uint64(bEnd)+1)
	return nil
}
