package console

import (
	"context"
	"fmt"
	"os"

	"github.com/jroimartin/gocui"
	"github.com/sirupsen/logrus"

	"github.com/harvester/harvester/pkg/installer/config"
	"github.com/harvester/harvester/pkg/installer/preflight"
	"github.com/harvester/harvester/pkg/installer/widgets"
)

var (
	debug bool
)

const (
	defaultLogFilePath = "/var/log/console.log"
)

func initLogs() error {
	if os.Getenv("DEBUG") == "true" {
		debug = true
		logrus.SetLevel(logrus.DebugLevel)
	}

	var logFilePath string
	if path := os.Getenv("LOGFILE"); path != "" {
		logFilePath = path
	} else {
		logFilePath = defaultLogFilePath
	}

	f, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600) //nolint:gosec
	if err != nil {
		return err
	}
	logrus.SetOutput(f)
	return nil
}

// Console is the structure of the harvester console
type Console struct {
	context context.Context
	*gocui.Gui
	elements map[string]widgets.Element
	config   *config.HarvesterConfig
}

// RunConsole starts the console
func RunConsole() error {
	c, err := NewConsole()
	if err != nil {
		return err
	}
	if err := initLogs(); err != nil {
		return err
	}
	err = c.doRun()
	if err != nil {
		// This ensures difficult to debug failures
		// (e.g. invalid dimensions) are actually logged
		logrus.Errorf("console.doRun() failed: %v", err)
	}
	return err
}

// NewConsole initialize the console
func NewConsole() (*Console, error) {
	g, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		return nil, err
	}
	return &Console{
		context:  context.Background(),
		Gui:      g,
		elements: make(map[string]widgets.Element),
		config:   config.NewHarvesterConfig(),
	}, nil
}

// GetElement gets an element by name
func (c *Console) GetElement(name string) (widgets.Element, error) {
	e, ok := c.elements[name]
	if ok {
		return e, nil
	}
	return nil, fmt.Errorf("element %q is not found", name)
}

// AddElement adds an element with name
func (c *Console) AddElement(name string, element widgets.Element) {
	c.elements[name] = element
}

// ShowElement shows the element by name
func (c *Console) ShowElement(name string) error {
	elem, err := c.GetElement(name)
	if err != nil {
		return err
	}
	return elem.Show()
}

func (c *Console) setContentByName(name string, content string) error {
	v, err := c.GetElement(name)
	if err != nil {
		return err
	}
	if content == "" {
		return v.Close()
	}
	if err := v.Show(); err != nil {
		return err
	}
	v.SetContent(content)
	_, err = c.Gui.SetViewOnTop(name)
	return err
}

func (c *Console) CloseElement(name string) {
	v, err := c.GetElement(name)
	if err != nil {
		return
	}
	if err = v.Close(); err != nil && err != gocui.ErrUnknownView {
		logrus.Error(err)
	}
}

func (c *Console) CloseElements(names ...string) {
	for _, name := range names {
		c.CloseElement(name)
	}
}

func (c *Console) doRun() error {
	defer c.Close()

	dashboard := c.layoutInstall
	preflightCheck := true

	if hd, _ := os.LookupEnv("HARVESTER_DASHBOARD"); hd == "true" {
		if err := c.getHarvesterConfig(); err != nil {
			return err
		}
		if c.config.Install.Mode == config.ModeCreate || c.config.Install.Mode == config.ModeJoin {
			dashboard = c.layoutDashboard
			// no need to do preflight check after the node is installed, it runs layoutDashboard directly
			// preflightWarnings are used in layoutInstall
			preflightCheck = false
		}
	}

	// installModeBoot is used to control options in layoutInstall
	if c.config.Install.Mode == config.ModeInstall {
		logrus.Info("harvester already installed")
		alreadyInstalled = true
		c.config.Install.Mode = ""
		preflightCheck = false
	}

	if preflightCheck {
		checks := []preflight.Check{
			preflight.BIOSCheck{},
			preflight.CPUCheck{},
			preflight.MemoryCheck{},
			preflight.VirtCheck{},
			preflight.KVMHostCheck{},
		}
		for _, c := range checks {
			msg, err := c.Run()
			if err != nil {
				// Preflight checks that fail to run at all are
				// logged, rather than killing the installer
				logrus.Error(err)
				continue
			}
			if len(msg) > 0 {
				preflightWarnings = append(preflightWarnings, msg)
			}
		}
	}

	c.SetManagerFunc(dashboard)

	if err := setGlobalKeyBindings(c.Gui); err != nil {
		return err
	}

	if err := c.MainLoop(); err != nil && err != gocui.ErrQuit {
		return err
	}
	return nil
}

func setGlobalKeyBindings(g *gocui.Gui) error {
	g.InputEsc = true
	if debug {
		if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
			return err
		}
	}
	return nil
}

func quit(_ *gocui.Gui, _ *gocui.View) error {
	return gocui.ErrQuit
}
