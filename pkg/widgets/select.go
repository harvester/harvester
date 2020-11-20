package widgets

import (
	"fmt"
	"strings"

	"github.com/jroimartin/gocui"
)

type Option struct {
	Value string
	Text  string
}

type GetOptionsFunc func() ([]Option, error)

type Select struct {
	*Panel

	getOptionsFunc GetOptionsFunc
	options        []Option
	optionV        *gocui.View
}

func NewSelect(g *gocui.Gui, name string, text string, getOptionsFunc GetOptionsFunc) (*Select, error) {
	return &Select{
		Panel: &Panel{
			Name:    name,
			g:       g,
			Content: text,
		},
		getOptionsFunc: getOptionsFunc,
	}, nil
}

func (s *Select) Show() error {
	var err error
	if err := s.Panel.Show(); err != nil {
		return err
	}
	if s.getOptionsFunc != nil {
		if s.options, err = s.getOptionsFunc(); err != nil {
			return err
		}
	}
	optionViewName := s.Name + "-options"
	offset := len(strings.Split(s.Content, "\n"))
	y0 := s.Y0 + offset - 1
	y1 := s.Y0 + offset + len(s.options) + 2
	v, err := s.g.SetView(optionViewName, s.X0, y0, s.X1, y1)
	if err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Highlight = true
		v.SelBgColor = gocui.ColorGreen
		v.SelFgColor = gocui.ColorBlack
		v.Wrap = true
		for _, opt := range s.options {
			if _, err := fmt.Fprintln(v, opt.Text); err != nil {
				return err
			}
		}

		if _, err := s.g.SetCurrentView(optionViewName); err != nil {
			return err
		}
		if err := setOptionsKeyBindings(s.g, optionViewName); err != nil {
			return err
		}
		if s.KeyBindings != nil {
			for key, f := range s.KeyBindings {
				if err := s.g.SetKeybinding(optionViewName, key, gocui.ModNone, f); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (s *Select) Close() error {
	optionViewName := s.Name + "-options"
	s.g.DeleteKeybindings(optionViewName)
	if err := s.g.DeleteView(optionViewName); err != nil {
		return err
	}
	return s.Panel.Close()
}

func (s *Select) GetData() (string, error) {
	optionViewName := s.Name + "-options"
	ov, err := s.g.View(optionViewName)
	if err != nil {
		return "", err
	}
	if len(ov.BufferLines()) == 0 {
		return "", nil
	}
	_, cy := ov.Cursor()
	var value string
	if len(s.options) >= cy+1 {
		value = s.options[cy].Value
	}
	return value, nil
}

func setOptionsKeyBindings(g *gocui.Gui, viewName string) error {
	if err := g.SetKeybinding(viewName, gocui.KeyArrowUp, gocui.ModNone, ArrowUp); err != nil {
		return err
	}
	if err := g.SetKeybinding(viewName, gocui.KeyArrowDown, gocui.ModNone, ArrowDown); err != nil {
		return err
	}
	return nil
}
