package widgets

import (
	"fmt"

	"github.com/jroimartin/gocui"
)

type Panel struct {
	g       *gocui.Gui
	Name    string
	Title   string
	Frame   bool
	Wrap    bool
	Focus   bool
	FgColor gocui.Attribute
	Content string
	X0      int
	X1      int
	Y0      int
	Y1      int

	// Hook functions
	PreShow   func() error
	PostClose func() error

	KeyBindings map[gocui.Key]func(*gocui.Gui, *gocui.View) error
}

func NewPanel(g *gocui.Gui, name string) *Panel {
	return &Panel{
		g:     g,
		Name:  name,
		Focus: true,
	}
}

func (p *Panel) GetName() string {
	return p.Name
}

func (p *Panel) Close() error {
	if _, err := p.g.View(p.Name); err == gocui.ErrUnknownView {
		return nil
	} else if err != nil {
		return err
	}
	p.g.DeleteKeybindings(p.Name)
	if err := p.g.DeleteView(p.Name); err != nil {
		return err
	}

	if p.PostClose != nil {
		if err := p.PostClose(); err != nil {
			return err
		}
	}
	return nil
}

func (p *Panel) Show() error {
	if p.PreShow != nil {
		if err := p.PreShow(); err != nil {
			return err
		}
	}
	if p.X0 == 0 && p.X1 == 0 && p.Y0 == 0 && p.Y1 == 0 {
		maxX, maxY := p.g.Size()
		p.X0 = maxX / 4
		p.X1 = maxX / 4 * 3
		p.Y0 = maxY / 4
		p.Y1 = maxY / 4 * 3
	}
	v, err := p.g.SetView(p.Name, p.X0, p.Y0, p.X1, p.Y1)
	if err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Title = p.Title
		v.Frame = p.Frame
		v.Wrap = p.Wrap
		v.FgColor = p.FgColor
		if _, err := fmt.Fprint(v, p.Content); err != nil {
			return err
		}

		if p.Focus {
			if _, err := p.g.SetCurrentView(p.Name); err != nil {
				return err
			}
		}
		for k, f := range p.KeyBindings {
			if err := p.g.SetKeybinding(p.Name, k, gocui.ModNone, f); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *Panel) SetLocation(x0, y0, x1, y1 int) {
	p.X0 = x0
	p.Y0 = y0
	p.X1 = x1
	p.Y1 = y1
}

func (p *Panel) SetContent(content string) {
	p.Content = content
	p.g.Update(func(g *gocui.Gui) error {
		v, err := p.g.View(p.Name)
		if err != nil && err != gocui.ErrUnknownView {
			return err
		}
		v.Clear()
		_, err = fmt.Fprint(v, p.Content)
		return err
	})
}

func (p *Panel) GetData() (string, error) {
	return p.Content, nil
}
