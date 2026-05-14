package widgets

import (
	"github.com/jroimartin/gocui"
)

type Input struct {
	*Panel
	Value string
	Mask  bool
}

func NewInput(g *gocui.Gui, name string, label string, mask bool) (*Input, error) {
	maxX, maxY := g.Size()
	return &Input{
		Panel: &Panel{
			Name:    name,
			g:       g,
			Content: label,
			X0:      maxX / 8,
			Y0:      maxY / 8,
			X1:      maxX / 8 * 7,
			Y1:      maxY/8 + 3,
		},
		Mask: mask,
	}, nil
}

func (i *Input) Show() error {
	if err := i.Panel.Show(); err != nil {
		return err
	}
	inputViewName := i.Name + "-input"
	offset := 20
	if len(i.Content) > offset {
		offset = len(i.Content) + 1
	}
	x0 := i.X0 + offset
	x1 := i.X1 - 1
	y0 := i.Y0
	y1 := i.Y0 + 2
	v, err := i.g.SetView(inputViewName, x0, y0, x1, y1)
	if err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Editable = true
		v.SelBgColor = gocui.ColorGreen
		v.SelFgColor = gocui.ColorBlack
		if i.Mask {
			v.Mask ^= '*'
		}
		if i.KeyBindings != nil {
			for key, f := range i.KeyBindings {
				if err := i.g.SetKeybinding(inputViewName, key, gocui.ModNone, f); err != nil {
					return err
				}
			}
		}
	}
	if _, err = i.g.SetCurrentView(inputViewName); err != nil {
		return err
	}

	return i.SetData(i.Value)
}

func (i *Input) Close() error {
	inputViewName := i.Name + "-input"
	// ov, err := i.g.View(inputViewName)
	// if err != nil {
	// 	return err
	// }
	// ov.Frame = false
	// ov.Clear()
	i.g.DeleteKeybindings(inputViewName)
	if err := i.g.DeleteView(inputViewName); err != nil {
		return err
	}
	return i.Panel.Close()
}

func (i *Input) GetData() (string, error) {
	inputViewName := i.Name + "-input"
	ov, err := i.g.View(inputViewName)
	if err != nil {
		return "", err
	}
	if len(ov.BufferLines()) == 0 {
		return "", nil
	}
	return ov.Line(0)
}

func (i *Input) SetData(data string) error {
	inputViewName := i.Name + "-input"
	ov, err := i.g.View(inputViewName)
	if err != nil {
		return err
	}
	ov.Clear()
	_, err = ov.Write([]byte(data))
	return err
}
