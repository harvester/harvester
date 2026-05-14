package console

import (
	"fmt"
	"time"

	"github.com/jroimartin/gocui"
)

type Spinner struct {
	g      *gocui.Gui
	panel  string
	prefix string
	ticker *time.Ticker
	focus  bool

	stop    chan TaskResult
	stopped chan bool
}

type TaskResult struct {
	err     bool
	message string
}

const (
	infoColor    = gocui.ColorCyan
	errorColor   = gocui.ColorRed
	spinInterval = 100 * time.Millisecond
)

func NewSpinner(g *gocui.Gui, panel string, prefix string) *Spinner {
	return &Spinner{
		g:       g,
		panel:   panel,
		prefix:  prefix,
		stop:    make(chan TaskResult),
		stopped: make(chan bool),
	}
}

func NewFocusSpinner(g *gocui.Gui, panel string, prefix string) *Spinner {
	return &Spinner{
		g:       g,
		panel:   panel,
		prefix:  prefix,
		stop:    make(chan TaskResult),
		stopped: make(chan bool),
		focus:   true,
	}
}

func (s *Spinner) Start() {
	s.ticker = time.NewTicker(spinInterval)
	go func() {
		s.writePanel(fmt.Sprintf("%s|", s.prefix), true, infoColor)
		for {
			for _, symbol := range `\-/|` {
				select {
				case r := <-s.stop:
					color := infoColor
					if r.err {
						color = errorColor
					}
					s.writePanel(r.message, true, color)
					s.stopped <- true
					return
				case <-s.ticker.C:
					s.writePanel(fmt.Sprintf("\r%s%c", s.prefix, symbol), false, infoColor)
				}
			}
		}
	}()
}

func (s *Spinner) Stop(err bool, message string) {
	s.ticker.Stop()
	s.stop <- TaskResult{err: err, message: message}
	<-s.stopped
}

func (s *Spinner) writePanel(message string, clearView bool, fgColor gocui.Attribute) {
	// g.Update spawns a goroutine to notify gocui to update.
	// wait until cui consumes the notification to make sure any remaining
	// writePanel calls don't go first
	ch := make(chan struct{})

	s.g.Update(func(g *gocui.Gui) error {
		defer func() {
			ch <- struct{}{}
		}()
		v, err := g.View(s.panel)
		if err == gocui.ErrUnknownView {
			return nil
		}
		if err != nil {
			return err
		}

		if s.focus {
			if _, err = g.SetCurrentView(s.panel); err != nil {
				return err
			}
		}
		if clearView {
			v.Clear()
		}
		v.FgColor = fgColor
		_, err = v.Write([]byte(message))
		return err
	})

	<-ch
}
