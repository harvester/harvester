package remotedialer

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	MaxBuffer = 1 << 21
)

type readBuffer struct {
	cond         sync.Cond
	deadline     time.Time
	buf          bytes.Buffer
	err          error
	backPressure *backPressure
}

func newReadBuffer(backPressure *backPressure) *readBuffer {
	return &readBuffer{
		backPressure: backPressure,
		cond: sync.Cond{
			L: &sync.Mutex{},
		},
	}
}

func (r *readBuffer) Offer(reader io.Reader) error {
	r.cond.L.Lock()
	defer r.cond.L.Unlock()

	if r.err != nil {
		return r.err
	}

	if n, err := io.Copy(&r.buf, reader); err != nil {
		return err
	} else if n > 0 {
		r.cond.Broadcast()
	}

	if r.buf.Len() > MaxBuffer {
		if err := r.backPressure.Pause(); err != nil {
			return err
		}
	}

	if r.buf.Len() > MaxBuffer*2 {
		logrus.Errorf("remotedialer buffer exceeded, length: %d", r.buf.Len())
	}

	return nil
}

func (r *readBuffer) Read(b []byte) (int, error) {
	r.cond.L.Lock()
	defer r.cond.L.Unlock()

	for {
		var (
			n   int
			err error
		)

		if r.buf.Len() > 0 {
			n, err = r.buf.Read(b)
			r.cond.Broadcast()
			if err != io.EOF {
				if r.buf.Len() < MaxBuffer/8 {
					if err := r.backPressure.Resume(); err != nil {
						return n, err
					}
				}
				return n, err
			}
			// buffer remains to be read
			if r.err == io.EOF && r.buf.Len() > 0 {
				return n, nil
			}
			return n, r.err
		}

		if r.err != nil {
			return n, r.err
		}

		now := time.Now()
		if !r.deadline.IsZero() {
			if now.After(r.deadline) {
				return 0, errors.New("deadline exceeded")
			}
		}

		var t *time.Timer
		if !r.deadline.IsZero() {
			t = time.AfterFunc(r.deadline.Sub(now), func() { r.cond.Broadcast() })
		}
		r.cond.Wait()
		if t != nil {
			t.Stop()
		}
	}
}

func (r *readBuffer) Close(err error) error {
	r.cond.L.Lock()
	defer r.cond.L.Unlock()
	if r.err == nil {
		r.err = err
	}
	r.cond.Broadcast()
	return nil
}
