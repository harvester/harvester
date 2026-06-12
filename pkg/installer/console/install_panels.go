package console

import (
	"fmt"
	"net"
	"net/netip"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/jroimartin/gocui"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/harvester/harvester/pkg/installer/config"
	"github.com/harvester/harvester/pkg/installer/preflight"
	"github.com/harvester/harvester/pkg/installer/util"
	"github.com/harvester/harvester/pkg/installer/version"
	"github.com/harvester/harvester/pkg/installer/widgets"
)

type UserInputData struct {
	ServerURL            string
	SSHKeyURL            string
	Password             string
	PasswordConfirm      string
	Address              string
	AddrMask             string
	DNSServers           string
	NTPServers           string
	HasCheckedNTPServers bool
}

type IPMask = net.IPMask
type ParseError = net.ParseError

const (
	NICStateNotFound = iota
	NICStateDown
	NICStateLowerDown
	NICStateUP
)

const (
	ErrMsgVLANShouldBeANumberInRange string = "VLAN ID should be a number 1 ~ 4094."
	ErrMsgMTUShouldBeANumber         string = "MTU should be a number."
	NtpSettingName                   string = "ntp-servers"
	ErrMsgNoDefaultRoute             string = "No default route found. Please check the router setting on the DHCP server."
)

var (
	once          sync.Once
	userInputData = UserInputData{
		NTPServers: "0.suse.pool.ntp.org",
	}
	mgmtNetwork = config.Network{
		DefaultRoute: true,
	}
	alreadyInstalled  bool
	installModeOnly   bool
	diskConfirmed     bool
	preflightWarnings []string
	preflightAck      bool
	diskOptionsCache  *DiskOptionsCache = NewDiskOptionsCache()
)

func (c *Console) doNetworkSpeedCheck(interfaces []config.NetworkInterface) (warnings []string) {
	for _, iface := range interfaces {
		msg, err := preflight.NetworkSpeedCheck{Dev: iface.Name}.Run()
		if err != nil {
			// Preflight checks that fail to run at all are
			// logged, rather than killing the installer
			logrus.Error(err)
			continue
		}
		if len(msg) > 0 {
			warnings = append(warnings, msg)
		}
	}
	return
}

func (c *Console) layoutInstall(_ *gocui.Gui) error {
	var err error
	once.Do(func() {
		if err = setPanels(c); err != nil {
			return
		}
		initPanel := askCreatePanel

		// If there's any preflight warnings, show those first.
		if len(preflightWarnings) > 0 {
			initPanel = preflightCheckPanel
		}

		c.config.OS.Modules = []string{"kvm", "vhost_net"}

		// if already installed then lets check if cloud init allows us to provision
		if alreadyInstalled {
			err = mergeCloudInit(c.config)
			if err != nil {
				logrus.Errorf("error merging cloud-config")
			}
			logrus.Infof("already install value post config merge: %v", c.config.Automatic)
			// if already installed and automatic installation is set to true
			// configure node directly
			if alreadyInstalled && c.config.Automatic {
				initPanel = installPanel
			}
		}

		if cfg, err := config.ReadConfig(); err == nil {
			if err = c.config.Merge(cfg); err != nil {
				logrus.Errorf("error merging config file: %v", err)
				return
			}
			if cfg.Install.Automatic && isFirstConsoleTTY() {
				logrus.Info("Start automatic installation...")
				// setup InstallMode to ensure that during automatic install
				// we are only copying binaries and ignoring network / rancherd setup
				// needed for generating pre-installed qcow2 image
				if c.config.Install.Mode == config.ModeInstall && !alreadyInstalled {
					installModeOnly = true
				}
				initPanel = installPanel
			}
		} else {
			logrus.Errorf("automatic install failed: %v\n", err)
		}

		// add SchemeVersion in non-automatic mode
		// in automatic mode, SchemeVersion should be from config.yaml directly
		if !c.config.Install.Automatic {
			c.config.SchemeVersion = config.SchemeVersion
		}

		initElements := []string{
			titlePanel,
			validatorPanel,
			notePanel,
			footerPanel,
			initPanel,
		}
		var e widgets.Element
		for _, name := range initElements {
			e, err = c.GetElement(name)
			if err != nil {
				return
			}
			if err = e.Show(); err != nil {
				return
			}
		}
	})
	return err
}

func setPanels(c *Console) error {
	funcs := []func(*Console) error{
		addTitlePanel,
		addValidatorPanel,
		addNotePanel,
		addFooterPanel,
		addPreflightCheckPanel,
		addAskCreatePanel,
		addAskRolePanel,
		addDiskPanel,
		addHostnamePanel,
		addNetworkPanel,
		addClusterNetworkPanel,
		addVIPPanel,
		addDNSServersPanel,
		addNTPServersPanel,
		addServerURLPanel,
		addTokenPanel,
		addPasswordPanels,
		addSSHPanel,
		addProxyPanel,
		addCloudInitPanel,
		addConfirmInstallPanel,
		addConfirmUpgradePanel,
		addInstallPanel,
		addSpinnerPanel,
		addUpgradePanel,
	}
	for _, f := range funcs {
		if err := f(c); err != nil {
			return err
		}
	}
	return nil
}

func addTitlePanel(c *Console) error {
	maxX, maxY := c.Gui.Size()
	titleV := widgets.NewPanel(c.Gui, titlePanel)
	titleV.SetLocation(maxX/8, maxY/8-3, maxX/8*7, maxY/8)
	titleV.Focus = false
	c.AddElement(titlePanel, titleV)
	return nil
}

func addValidatorPanel(c *Console) error {
	maxX, maxY := c.Gui.Size()
	validatorV := widgets.NewPanel(c.Gui, validatorPanel)
	validatorV.SetLocation(maxX/8, maxY/8+5, maxX/8*7, maxY/8*7)
	validatorV.FgColor = gocui.ColorRed
	validatorV.Wrap = true
	validatorV.Focus = false
	c.AddElement(validatorPanel, validatorV)
	return nil
}

func addNotePanel(c *Console) error {
	maxX, maxY := c.Gui.Size()
	noteV := widgets.NewPanel(c.Gui, notePanel)
	noteV.SetLocation(maxX/8, maxY/8+3, maxX/8*7, maxY/8+6)
	noteV.Wrap = true
	noteV.Focus = false
	c.AddElement(notePanel, noteV)
	return nil
}

func addFooterPanel(c *Console) error {
	maxX, maxY := c.Gui.Size()
	footerV := widgets.NewPanel(c.Gui, footerPanel)
	footerV.SetLocation(0, maxY-2, maxX, maxY)
	footerV.Focus = false
	c.AddElement(footerPanel, footerV)
	return nil
}

func showDiskPage(c *Console) error {
	diskConfirmed = false

	if err := diskOptionsCache.refresh(); err != nil {
		return err
	}

	diskOptions := diskOptionsCache.getAllValidDiskOptions()

	presetConfigDisks(c, diskOptions)

	if len(diskOptions) == 0 {
		return showNext(c, diskFatalPanel)
	}

	showPersistentSizeOption := c.config.Install.Role != config.RoleWitness &&
		(len(diskOptions) == 1 || c.config.Install.DataDisk == c.config.Install.Device)

	nextComponents := []string{diskPanel}
	if c.config.Install.Role != config.RoleWitness &&
		len(diskOptions) > 1 {
		nextComponents = append([]string{dataDiskPanel}, nextComponents...)
	}

	if showPersistentSizeOption {
		nextComponents = append([]string{persistentSizePanel}, nextComponents...)
	}
	return showNext(c, nextComponents...)
}

func calculateDefaultPersistentSize(dev string) (string, error) {
	bytes, err := util.GetDiskSizeBytes(dev)
	if err != nil {
		return "", err
	}

	defaultBytes := uint64(float64(bytes) * config.DefaultPersistentPercentageNum)
	defaultSize := util.ByteToGi(defaultBytes)
	if defaultSize < config.PersistentSizeMinGiB {
		defaultSize = config.PersistentSizeMinGiB
	}
	return fmt.Sprintf("%dGi", defaultSize), nil
}

func presetConfigDisks(c *Console, diskOpts []widgets.Option) {
	if c.config.Install.Device != "" && c.config.Install.DataDisk != "" {
		return
	}
	if len(diskOpts) == 0 {
		return
	}

	if c.config.Install.Device == "" {
		c.config.Install.Device = diskOpts[0].Value
	}
	if c.config.Install.DataDisk == "" {
		c.config.Install.DataDisk = c.config.Install.Device
	}
}

func addDiskPanel(c *Console) error {
	diskConfirmed = false

	setLocation := createVerticalLocator(c)

	setPageTitle := func() error {
		if c.config.Install.Role == config.RoleWitness {
			return c.setContentByName(titlePanel, "Choose installation target. Device will be formatted")
		}
		return c.setContentByName(titlePanel, "Choose installation target and data disk. Device will be formatted")
	}
	closeThisPage := func() {
		c.CloseElements(
			diskFatalPanel,
			diskPanel,
			dataDiskPanel,
			diskValidatorPanel,
			diskNotePanel,
			persistentSizePanel,
			wipeDisksTitlePanel,
			wipeDisksPanel,
		)
	}
	gotoPrevPage := func(_ *gocui.Gui, _ *gocui.View) error {
		closeThisPage()
		diskConfirmed = false
		return showPasswordPage(c)
	}

	diskFatalV := widgets.NewPanel(c.Gui, diskFatalPanel)
	diskFatalV.FgColor = gocui.ColorRed
	diskFatalV.Wrap = true
	diskFatalV.PreShow = func() error {
		diskFatalV.SetContent("No disk detected. Harvester requires at least one disk.")
		return setPageTitle()
	}
	setLocation(diskFatalV, 3)
	c.AddElement(diskFatalPanel, diskFatalV)
	diskFatalV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyEsc: gotoPrevPage,
	}

	// Select device panel
	diskV, err := widgets.NewDropDown(c.Gui, diskPanel, diskLabel, func() ([]widgets.Option, error) {
		return diskOptionsCache.getAllValidDiskOptions(), nil
	})
	if err != nil {
		return err
	}
	diskV.PreShow = func() error {
		if err := diskV.SetData(c.config.Install.Device); err != nil {
			return err
		}

		if err := c.setContentByName(diskNotePanel, ""); err != nil {
			return err
		}
		return setPageTitle()
	}
	setLocation(diskV.Panel, 3)
	c.AddElement(diskPanel, diskV)

	dataDiskV, err := widgets.NewDropDown(c.Gui, dataDiskPanel, dataDiskLabel, func() ([]widgets.Option, error) {
		return diskOptionsCache.getDataDiskOptions(c.config), nil
	})
	if err != nil {
		return err
	}

	dataDiskV.PreShow = func() error {
		return dataDiskV.SetData(c.config.Install.DataDisk)
	}
	setLocation(dataDiskV.Panel, 3)
	c.AddElement(dataDiskPanel, dataDiskV)

	// Persistent partition size panel
	persistentSizeV, err := widgets.NewInput(c.Gui, persistentSizePanel, persistentSizeLabel, false)
	if err != nil {
		return err
	}
	persistentSizeV.PreShow = func() error {
		c.Cursor = true

		device := c.config.Install.Device
		//If the user has already set a persistent partition size, use that
		if persistentSizeV.Value != "" {
			if c.config.Install.PersistentPartitionSize != "" {
				persistentSizeV.Value = c.config.Install.PersistentPartitionSize
			} else {
				defaultValue, err := calculateDefaultPersistentSize(device)
				if err != nil {
					return err
				}
				persistentSizeV.Value = defaultValue
			}
		} else {
			defaultValue, err := calculateDefaultPersistentSize(device)
			if err != nil {
				return err
			}
			persistentSizeV.Value = defaultValue
		}
		return nil
	}
	setLocation(persistentSizeV, 3)
	c.AddElement(persistentSizePanel, persistentSizeV)

	// WipeDisksPanel
	wipeDisksTitlePanelV := widgets.NewPanel(c.Gui, wipeDisksTitlePanel)
	wipeDisksTitlePanelV.SetContent("Additional Harvester installations detected")
	wipeDisksTitlePanelV.FgColor = gocui.ColorRed
	setLocation(wipeDisksTitlePanelV, 3)
	c.AddElement(wipeDisksTitlePanel, wipeDisksTitlePanelV)

	wipeDisksV, err := widgets.NewDropDown(c.Gui, wipeDisksPanel, wipeDisksLabel, func() ([]widgets.Option, error) {
		return diskOptionsCache.getWipeDisksOptions(c.config), nil
	})
	if err != nil {
		return err
	}
	setLocation(wipeDisksV.Panel, 3)
	wipeDisksV.Autoscroll = true
	wipeDisksV.FgColor = gocui.ColorRed
	wipeDisksV.SetMulti(true) // allow multiple disks to be selected for wipe operation
	wipeDisksV.PreShow = func() error {
		wipeDisksV.Focus = true
		return nil
	}
	c.AddElement(wipeDisksPanel, wipeDisksV)

	// Note panel for persistent partition size
	diskNoteV := widgets.NewPanel(c.Gui, diskNotePanel)
	diskNoteV.Wrap = true
	setLocation(diskNoteV, 3)
	c.AddElement(diskNotePanel, diskNoteV)

	// Panel for showing validator message
	diskValidatorV := widgets.NewPanel(c.Gui, diskValidatorPanel)
	diskValidatorV.FgColor = gocui.ColorRed
	diskValidatorV.Wrap = true
	updateValidatorMessage := func(msg string) error {
		diskValidatorV.Focus = false
		return c.setContentByName(diskValidatorPanel, msg)
	}
	setLocation(diskValidatorV, 3)
	c.AddElement(diskValidatorPanel, diskValidatorV)

	// Helper functions
	validateAllDiskSizes := func() (bool, error) {
		installDisk := c.config.Install.Device
		dataDisk := c.config.Install.DataDisk
		if c.config.SkipChecks {
			return true, nil
		}

		if c.config.Install.Role == config.RoleWitness {
			if err := validateDiskSize(installDisk, false); err != nil {
				return false, updateValidatorMessage(err.Error())
			}
		} else if dataDisk == "" || installDisk == dataDisk {
			if err := validateDiskSize(installDisk, true); err != nil {
				return false, updateValidatorMessage(err.Error())
			}
		} else {
			if err := validateDiskSize(installDisk, false); err != nil {
				return false, updateValidatorMessage(err.Error())
			}
			if err := validateDataDiskSize(dataDisk); err != nil {
				return false, updateValidatorMessage(err.Error())
			}
		}
		return true, nil
	}
	validatePersistentPartitionSize := func(persistentSize string) (bool, error) {
		installDisk := c.config.Install.Device
		dataDisk := c.config.Install.DataDisk
		if dataDisk == "" || installDisk == dataDisk {
			diskSize, err := util.GetDiskSizeBytes(installDisk)
			if err != nil {
				return false, err
			}
			if _, err := util.ParsePartitionSize(diskSize, persistentSize, c.config.SkipChecks); err != nil {
				return false, updateValidatorMessage(err.Error())
			}
		}
		return true, nil
	}
	gotoNextPage := func(_ *gocui.Gui, _ *gocui.View) error {
		// Don't proceed to the next page if disk size validation fails
		if valid, err := validateAllDiskSizes(); !valid || err != nil {
			return err
		}

		if c.config.Install.Role == config.RoleWitness {
			c.config.Install.DataDisk = ""
			if err := dataDiskV.SetData(""); err != nil {
				return err
			}
		} else {
			// Make sure the persistent partition size is in the correct size.
			// Do NOT allow proceeding to next field.
			if valid, err := validatePersistentPartitionSize(c.config.Install.PersistentPartitionSize); !valid || err != nil {
				return err
			}
		}

		if !diskConfirmed {
			diskConfirmed = true
			return nil
		}

		closeThisPage()
		if installModeOnly {
			return showNext(c, confirmInstallPanel)
		}
		return showNetworkPage(c)
	}

	// isWipeDisksPanelNeeded is a helper function to render the wipeDisksPanel if needed
	// else will move on to the next apge
	isWipeDisksPanelNeeded := func(g *gocui.Gui, v *gocui.View) error {
		options := diskOptionsCache.getWipeDisksOptions(c.config)
		if len(options) != 0 {
			if slices.Contains(c.config.WipeDisksList, c.config.Device) || slices.Contains(c.config.WipeDisksList, c.config.DataDisk) {
				c.config.WipeDisksList = []string{}
				wipeDisksV.Reset()
			}
			return showNext(c, wipeDisksTitlePanel, wipeDisksPanel)
		}
		// no disks left to wipe, so close the wipeDisksTitlePanel and wipeDisksPanel
		c.CloseElements(wipeDisksTitlePanel, wipeDisksPanel)

		return gotoNextPage(g, v)
	}

	diskConfirm := func(g *gocui.Gui, v *gocui.View) error {
		device, err := diskV.GetData()
		if err != nil {
			return err
		}
		dataDisk, err := dataDiskV.GetData()
		if err != nil {
			return err
		}

		if err := updateValidatorMessage(""); err != nil {
			return err
		}
		c.config.Install.Device = device

		diskOpts := diskOptionsCache.getAllValidDiskOptions()
		if len(diskOpts) > 1 {
			// Show error if disk size validation fails, but allow proceeding to next field
			if _, err := validateAllDiskSizes(); err != nil {
				return err
			}
			if c.config.Install.Role == config.RoleWitness {
				return isWipeDisksPanelNeeded(g, v)
			}
			if device == dataDisk {
				return showNext(c, persistentSizePanel, dataDiskPanel)
			}
			c.CloseElements(persistentSizePanel)
			return showNext(c, dataDiskPanel)
		}

		if c.config.Install.Role != config.RoleWitness {
			// Only show this for non-witness nodes, because:
			// 1. witness nodes don't let you set persistent size, so the note makes no sense
			// 2. showing the note for witness nodes, with no other fields present ends up
			//    preventing key events and everything locks up
			if err := c.setContentByName(diskNotePanel, persistentSizeNote); err != nil {
				return err
			}
		}
		// Show error if disk size validation fails, but allow proceeding to next field
		valid, err := validateAllDiskSizes()
		if err != nil {
			return err
		}
		if c.config.Install.Role == config.RoleWitness {
			// Set diskConfirmed here to avoid having to press ENTER twice
			// to proceed if the disk configuration is valid
			diskConfirmed = valid
			return isWipeDisksPanelNeeded(g, v)
		}
		return showNext(c, persistentSizePanel)
	}

	// Keybindings
	diskV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyEnter:     diskConfirm,
		gocui.KeyArrowDown: diskConfirm,
		gocui.KeyArrowUp: func(_ *gocui.Gui, _ *gocui.View) error {
			return updateValidatorMessage("")
		},
		gocui.KeyEsc: gotoPrevPage,
	}

	dataDiskConfirm := func(g *gocui.Gui, v *gocui.View) error {
		dataDisk, err := dataDiskV.GetData()
		if err != nil {
			return err
		}

		if err := updateValidatorMessage(""); err != nil {
			return err
		}
		c.config.Install.DataDisk = dataDisk

		installDisk, err := diskV.GetData()
		if err != nil {
			return err
		}
		if installDisk == dataDisk {
			if err := c.setContentByName(diskNotePanel, persistentSizeNote); err != nil {
				return err
			}
			// Show error if disk size validation fails, but allow proceeding to next field
			if _, err := validateAllDiskSizes(); err != nil {
				return err
			}
			return showNext(c, persistentSizePanel)
		}

		c.CloseElements(persistentSizePanel)

		// Make sure disk settings are correct. Do NOT allow proceeding to
		// next field.
		if _, err := validateAllDiskSizes(); err != nil {
			return err
		}

		// At this point the disk configuration is valid.
		diskConfirmed = true

		return isWipeDisksPanelNeeded(g, v)
	}
	dataDiskV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyEnter:     dataDiskConfirm,
		gocui.KeyArrowDown: dataDiskConfirm,
		gocui.KeyArrowUp: func(_ *gocui.Gui, _ *gocui.View) error {
			if err := updateValidatorMessage(""); err != nil {
				return err
			}
			diskConfirmed = false
			return showNext(c, diskPanel)
		},
		gocui.KeyEsc: gotoPrevPage,
	}

	persistentSizeConfirm := func(g *gocui.Gui, v *gocui.View) error {
		persistentSize, err := persistentSizeV.GetData()
		if err != nil {
			return err
		}

		// Clear previous error message.
		if err := updateValidatorMessage(""); err != nil {
			return err
		}

		// Make sure that the specified size meets the requirements.
		if valid, err := validatePersistentPartitionSize(persistentSize); !valid || err != nil {
			return err
		}

		c.config.Install.PersistentPartitionSize = persistentSize

		// At this point the disk configuration is valid.
		diskConfirmed = true
		return isWipeDisksPanelNeeded(g, v)
	}

	persistentSizeV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyEnter: persistentSizeConfirm,
		gocui.KeyArrowUp: func(_ *gocui.Gui, _ *gocui.View) error {
			diskConfirmed = false
			diskOpts := diskOptionsCache.getAllValidDiskOptions()
			if len(diskOpts) > 1 {
				if err := updateValidatorMessage(""); err != nil {
					return err
				}
				if err := c.setContentByName(diskNotePanel, ""); err != nil {
					return err
				}
				return showNext(c, dataDiskPanel)
			}
			if err := updateValidatorMessage(""); err != nil {
				return err
			}
			if err := c.setContentByName(diskNotePanel, ""); err != nil {
				return err
			}
			return showNext(c, diskPanel)
		},
		gocui.KeyArrowDown: persistentSizeConfirm,
		gocui.KeyEsc:       gotoPrevPage,
	}

	wipeDisksConfirm := func(g *gocui.Gui, v *gocui.View) error {
		c.config.WipeDisksList = wipeDisksV.GetMultiData()
		return gotoNextPage(g, v)
	}

	wipeDisksV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyEnter:     wipeDisksConfirm,
		gocui.KeyArrowDown: wipeDisksConfirm,
		gocui.KeyArrowUp: func(_ *gocui.Gui, _ *gocui.View) error {
			diskConfirmed = false

			disk, err := diskV.GetData()
			if err != nil {
				return err
			}
			dataDisk, err := dataDiskV.GetData()
			if err != nil {
				return err
			}

			if c.config.Install.Role == config.RoleWitness {
				return showNext(c, diskPanel)
			}
			diskOpts := diskOptionsCache.getAllValidDiskOptions()
			if len(diskOpts) > 1 && disk != dataDisk {
				return showNext(c, dataDiskPanel)
			}
			if err := c.setContentByName(diskNotePanel, persistentSizeNote); err != nil {
				return err
			}
			return showNext(c, persistentSizePanel)
		},
		gocui.KeyEsc: gotoPrevPage,
	}
	return nil
}

func addPreflightCheckPanel(c *Console) error {
	ackWarningsFunc := func() ([]widgets.Option, error) {
		return []widgets.Option{
			{
				Value: "yes",
				Text:  "Yes",
			}, {
				Value: "no",
				Text:  "No",
			},
		}, nil
	}
	preflightCheckV, err := widgets.NewSelect(c.Gui, preflightCheckPanel, "", ackWarningsFunc)
	if err != nil {
		return err
	}
	preflightCheckV.FirstPage = true
	preflightCheckV.Wrap = true
	preflightCheckV.PreShow = func() error {
		var warnings string
		for _, w := range preflightWarnings {
			warnings += w + "\n"
		}
		preflightCheckV.SetContent(warnings +
			"\nDo you wish to proceed?\n")
		c.Gui.Cursor = false
		return c.setContentByName(titlePanel, "Hardware Checks")
	}
	preflightCheckV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyEnter: func(_ *gocui.Gui, _ *gocui.View) error {
			proceed, err := preflightCheckV.GetData()
			if err != nil {
				return err
			}
			if proceed == "no" {
				if err = preflightCheckV.Close(); err != nil {
					return err
				}
				if err = c.setContentByName(titlePanel, ""); err != nil {
					return err
				}
				if err = c.setContentByName(footerPanel, ""); err != nil {
					return err
				}
				go util.SleepAndReboot() //nolint:errcheck
				return c.setContentByName(notePanel, "Installation halted. Rebooting system in 5 seconds")
			}
			if err = preflightCheckV.Close(); err != nil {
				return err
			}
			return showNext(c, askCreatePanel)
		},
	}
	c.AddElement(preflightCheckPanel, preflightCheckV)
	return nil
}

func addAskCreatePanel(c *Console) error {
	askOptionsFunc := func() ([]widgets.Option, error) {
		options := []widgets.Option{
			{
				Value: config.ModeCreate,
				Text:  "Create a new Harvester cluster",
			}, {
				Value: config.ModeJoin,
				Text:  "Join an existing Harvester cluster",
			},
		}

		// layoutInstall is now called from layoutDashboard due to the addition
		// of the new config.ModeInstall. config will be setup by layoutDashboard before passing control here
		// this extra option should only show up if that is not the case
		if !alreadyInstalled {
			options = append(options, widgets.Option{
				Value: config.ModeInstall,
				Text:  "Install Harvester binaries only",
			})
		}
		return options, nil
	}
	// new cluster or join existing cluster
	askCreateV, err := widgets.NewSelect(c.Gui, askCreatePanel, "", askOptionsFunc)
	if err != nil {
		return err
	}
	askCreateV.FirstPage = true
	askCreateV.PreShow = func() error {
		// If we're in the interactive installer at this point, it means the
		// user wants the installation to succeed, regardless of whether any
		// of the initial preflight checks failed, or if the later network
		// speed check fails.
		preflightAck = true
		askCreateV.Value = c.config.Install.Mode
		if alreadyInstalled {
			return c.setContentByName(titlePanel, "Harvester already installed. Choose configuration mode")
		}
		return c.setContentByName(titlePanel, "Choose installation mode")
	}
	askCreateV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyEnter: func(_ *gocui.Gui, _ *gocui.View) error {
			selected, err := askCreateV.GetData()
			if err != nil {
				return err
			}

			if alreadyInstalled {
				// need to wipe the Mode value before we set panel
				// needed to ensure that value lookup fails as install
				// is not available in the option in this case
				c.config.Install.Mode = ""
			}

			c.config.Install.Mode = selected
			// explicitly set this false to ensure if user changes from
			// install mode only to create /join then the variable is
			// reset to ensure correct panel sequence is displayed
			installModeOnly = false
			if selected == config.ModeInstall {
				installModeOnly = true
			}

			if c.config.Install.Mode == config.ModeCreate {
				c.config.Install.Role = config.RoleDefault
			}

			if err = askCreateV.Close(); err != nil {
				return err
			}

			switch selected {
			case config.ModeCreate:
				c.config.ServerURL = ""
				userInputData.ServerURL = ""
			case config.ModeUpgrade:
				return showNext(c, confirmUpgradePanel)
			}

			// all packages are already install
			// configure hostname and network
			if c.config.Install.Mode == config.ModeJoin {
				return showRolePage(c)
			}
			if alreadyInstalled {
				return showNetworkPage(c)
			}
			return showPasswordPage(c)
		},
	}
	c.AddElement(askCreatePanel, askCreateV)
	return nil
}

func showPasswordPage(c *Console) error {
	return showNext(c, passwordConfirmPanel, passwordPanel)
}

func showRolePage(c *Console) error {
	setLocation := createVerticalLocatorWithName(c)

	if err := setLocation(askRolePanel, 3); err != nil {
		return err
	}

	return showNext(c, askRolePanel)
}

func addAskRolePanel(c *Console) error {
	askOptionsFunc := func() ([]widgets.Option, error) {
		return []widgets.Option{
			{
				Value: config.RoleDefault,
				Text:  "Default Role (Management or Worker)",
			}, {
				Value: config.RoleMgmt,
				Text:  "Management Role",
			}, {
				Value: config.RoleWitness,
				Text:  "Witness Role",
			}, {
				Value: config.RoleWorker,
				Text:  "Worker Role",
			},
		}, nil
	}
	askRoleV, err := widgets.NewSelect(c.Gui, askRolePanel, "", askOptionsFunc)
	if err != nil {
		return err
	}
	gotoPrevPage := func(_ *gocui.Gui, _ *gocui.View) error {
		c.CloseElements(askRolePanel)
		return showNext(c, askCreatePanel)
	}
	askRoleV.PreShow = func() error {
		askRoleV.Value = c.config.Install.Role
		return c.setContentByName(titlePanel, "Choose installation role")
	}
	askRoleV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyEnter: func(_ *gocui.Gui, _ *gocui.View) error {
			selected, err := askRoleV.GetData()
			if err != nil {
				return err
			}
			c.config.Install.Role = selected
			if err = askRoleV.Close(); err != nil {
				return err
			}
			if alreadyInstalled {
				return showNetworkPage(c)
			}
			return showPasswordPage(c)
		},
		gocui.KeyEsc: gotoPrevPage,
	}
	c.AddElement(askRolePanel, askRoleV)
	return nil
}

func addServerURLPanel(c *Console) error {
	serverURLV, err := widgets.NewInput(c.Gui, serverURLPanel, "Management address", false)
	if err != nil {
		return err
	}
	serverURLV.PreShow = func() error {
		c.Gui.Cursor = true
		serverURLV.Value = userInputData.ServerURL
		if err := c.setContentByName(titlePanel, "Configure management address"); err != nil {
			return err
		}
		return c.setContentByName(notePanel, serverURLNote)
	}
	serverURLV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyEnter: func(_ *gocui.Gui, _ *gocui.View) error {
			asyncTaskV, err := c.GetElement(spinnerPanel)
			if err != nil {
				return err
			}
			if err = asyncTaskV.Close(); err != nil {
				return err
			}

			userInputData.ServerURL, err = serverURLV.GetData()
			if err != nil {
				return err
			}

			if userInputData.ServerURL == "" {
				return c.setContentByName(validatorPanel, "Management address is required")
			}

			fmtServerURL, err := getFormattedServerURL(userInputData.ServerURL)
			if err != nil {
				return c.setContentByName(validatorPanel, err.Error())
			}
			c.CloseElement(validatorPanel)

			// focus on task panel to prevent input
			if err = asyncTaskV.Show(); err != nil {
				return err
			}

			pingServerURL := fmtServerURL + "/ping"
			spinner := NewSpinner(c.Gui, spinnerPanel, fmt.Sprintf("Checking %q...", pingServerURL))
			spinner.Start()
			go func(g *gocui.Gui) {
				if err = validatePingServerURL(pingServerURL); err != nil {
					spinner.Stop(true, err.Error())
					g.Update(func(_ *gocui.Gui) error {
						return showNext(c, serverURLPanel)
					})
					return
				}
				spinner.Stop(false, "")
				c.config.ServerURL = fmtServerURL
				g.Update(func(_ *gocui.Gui) error {
					if err := serverURLV.Close(); err != nil {
						return err
					}
					return showNext(c, tokenPanel)
				})
			}(c.Gui)
			return nil
		},
		gocui.KeyEsc: func(g *gocui.Gui, _ *gocui.View) error {
			g.Cursor = false
			if err := serverURLV.Close(); err != nil {
				return err
			}
			return showNext(c, dnsServersPanel)
		},
	}
	serverURLV.PostClose = func() error {
		asyncTaskV, err := c.GetElement(spinnerPanel)
		if err != nil {
			return err
		}
		return asyncTaskV.Close()
	}
	c.AddElement(serverURLPanel, serverURLV)
	return nil
}

func addPasswordPanels(c *Console) error {
	maxX, maxY := c.Gui.Size()
	passwordV, err := widgets.NewInput(c.Gui, passwordPanel, "Password", true)
	if err != nil {
		return err
	}
	passwordConfirmV, err := widgets.NewInput(c.Gui, passwordConfirmPanel, "Confirm password", true)
	if err != nil {
		return err
	}

	pw := &passwordWrapper{
		c:                c,
		passwordV:        passwordV,
		passwordConfirmV: passwordConfirmV,
	}

	pw.passwordV.PreShow = func() error {
		passwordV.Value = userInputData.Password
		return nil
	}

	pw.passwordV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyEnter:     pw.passwordVConfirmKeyBinding,
		gocui.KeyArrowDown: pw.passwordVConfirmKeyBinding,
		gocui.KeyEsc:       pw.passwordVEscapeKeyBinding,
	}

	pw.passwordV.SetLocation(maxX/8, maxY/8, maxX/8*7, maxY/8+2)
	c.AddElement(passwordPanel, pw.passwordV)

	pw.passwordConfirmV.PreShow = func() error {
		c.Gui.Cursor = true
		passwordConfirmV.Value = userInputData.PasswordConfirm
		if err := c.setContentByName(notePanel, ""); err != nil {
			return err
		}
		return c.setContentByName(titlePanel, "Password change: set the default login password")
	}
	pw.passwordConfirmV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyArrowUp: pw.passwordConfirmVArrowUpKeyBinding,
		gocui.KeyEnter:   pw.passwordConfirmVKeyEnter,
		gocui.KeyEsc:     pw.passwordConfirmVKeyEscape,
	}
	pw.passwordConfirmV.SetLocation(maxX/8, maxY/8+3, maxX/8*7, maxY/8+5)
	c.AddElement(passwordConfirmPanel, passwordConfirmV)

	return nil
}

func addSSHPanel(c *Console) error {
	setLocation := createVerticalLocator(c)

	sshKeyV, err := widgets.NewInput(c.Gui, sshKeyPanel, "HTTP URL", false)
	if err != nil {
		return err
	}

	sshPasswordAuthV, err := widgets.NewDropDown(c.Gui, sshPasswordAuthPanel, "Password auth", func() ([]widgets.Option, error) {
		return []widgets.Option{
			{Value: "true", Text: "Enabled"},
			{Value: "false", Text: "Disabled"},
		}, nil
	})
	if err != nil {
		return err
	}

	closeThisPage := func() error {
		c.CloseElement(sshPasswordAuthPanel)
		return sshKeyV.Close()
	}

	sshKeyV.PreShow = func() error {
		c.Gui.Cursor = true
		sshKeyV.Value = userInputData.SSHKeyURL
		if err = c.setContentByName(titlePanel, "Optional: configure SSH"); err != nil {
			return err
		}
		if err = c.setContentByName(notePanel, sshKeyNote); err != nil {
			return err
		}
		if len(c.config.SSHAuthorizedKeys) > 0 {
			if err = sshPasswordAuthV.Show(); err != nil {
				return err
			}
			c.Gui.Cursor = true
		}
		return nil
	}
	gotoNextPage := func() error {
		if len(c.config.SSHAuthorizedKeys) > 0 {
			return showNext(c, sshPasswordAuthPanel)
		}
		c.config.OS.SSHD.DisablePasswordAuth = false
		if err := closeThisPage(); err != nil {
			return err
		}
		return showNext(c, cloudInitPanel)
	}
	sshKeyV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyEnter: func(_ *gocui.Gui, _ *gocui.View) error {
			url, err := sshKeyV.GetData()
			if err != nil {
				return err
			}
			if url != "" && url == userInputData.SSHKeyURL && len(c.config.SSHAuthorizedKeys) > 0 {
				return gotoNextPage()
			}
			hadKeys := len(c.config.SSHAuthorizedKeys) > 0
			userInputData.SSHKeyURL = url
			c.config.SSHAuthorizedKeys = []string{}
			if url != "" {
				// focus on task panel to prevent ssh input
				asyncTaskV, err := c.GetElement(spinnerPanel)
				if err != nil {
					return err
				}
				if err = asyncTaskV.Close(); err != nil {
					return err
				}
				if err = asyncTaskV.Show(); err != nil {
					return err
				}

				spinner := NewSpinner(c.Gui, spinnerPanel, fmt.Sprintf("Checking %q...", url))
				spinner.Start()

				go func(g *gocui.Gui) {
					pubKeys, err := getRemoteSSHKeys(url)
					if err != nil {
						spinner.Stop(true, err.Error())
						g.Update(func(_ *gocui.Gui) error {
							return showNext(c, sshKeyPanel)
						})
						return
					}
					spinner.Stop(false, "")
					logrus.Debug("SSH public keys: ", pubKeys)
					c.config.SSHAuthorizedKeys = pubKeys
					g.Update(func(_ *gocui.Gui) error {
						return gotoNextPage()
					})
				}(c.Gui)
				return nil
			}
			if hadKeys {
				c.CloseElement(sshPasswordAuthPanel)
				return showNext(c, sshKeyPanel)
			}
			return gotoNextPage()
		},
		gocui.KeyEsc: func(_ *gocui.Gui, _ *gocui.View) error {
			if err := closeThisPage(); err != nil {
				return err
			}
			return showNext(c, proxyPanel)
		},
	}
	sshKeyV.PostClose = func() error {
		if err := c.setContentByName(notePanel, ""); err != nil {
			return err
		}
		asyncTaskV, err := c.GetElement(spinnerPanel)
		if err != nil {
			return err
		}
		return asyncTaskV.Close()
	}
	sshPasswordAuthV.PreShow = func() error {
		c.Gui.Cursor = false
		if sshPasswordAuthV.Value == "" {
			sshPasswordAuthV.Value = strconv.FormatBool(!c.config.OS.SSHD.DisablePasswordAuth)
		}
		return nil
	}
	sshPasswordAuthNavigate := func() error {
		selected, err := sshPasswordAuthV.GetData()
		if err != nil {
			return err
		}
		passwordAuthEnabled, err := strconv.ParseBool(selected)
		if err != nil {
			return err
		}
		c.config.OS.SSHD.DisablePasswordAuth = !passwordAuthEnabled
		if err := closeThisPage(); err != nil {
			return err
		}
		return showNext(c, cloudInitPanel)
	}
	sshPasswordAuthV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyEnter: func(_ *gocui.Gui, v *gocui.View) error {
			if v != nil && v.Name() != sshPasswordAuthV.ViewName {
				return showNext(c, sshPasswordAuthPanel)
			}
			return sshPasswordAuthNavigate()
		},
		gocui.KeyArrowDown: func(_ *gocui.Gui, _ *gocui.View) error {
			return sshPasswordAuthNavigate()
		},
		gocui.KeyArrowUp: func(_ *gocui.Gui, _ *gocui.View) error {
			c.CloseElement(sshPasswordAuthPanel)
			return showNext(c, sshKeyPanel)
		},
		gocui.KeyEsc: func(_ *gocui.Gui, _ *gocui.View) error {
			c.CloseElement(sshPasswordAuthPanel)
			return showNext(c, sshKeyPanel)
		},
	}

	setLocation(sshKeyV.Panel, 3)
	setLocation(sshPasswordAuthV.Panel, 3)

	c.AddElement(sshKeyPanel, sshKeyV)
	c.AddElement(sshPasswordAuthPanel, sshPasswordAuthV)
	return nil
}

func addTokenPanel(c *Console) error {
	tokenV, err := widgets.NewInput(c.Gui, tokenPanel, "Cluster token", false)
	if err != nil {
		return err
	}
	tokenV.PreShow = func() error {
		c.Gui.Cursor = true
		tokenV.Value = c.config.Token
		tokenNote := clusterTokenJoinNote
		if c.config.Install.Mode == config.ModeCreate {
			tokenNote = clusterTokenCreateNote
		}
		if err = c.setContentByName(notePanel, tokenNote); err != nil {
			return err
		}
		return c.setContentByName(titlePanel, "Configure cluster token")
	}
	closeThisPage := func() error {
		c.CloseElement(notePanel)
		return tokenV.Close()
	}
	tokenV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyEnter: func(_ *gocui.Gui, _ *gocui.View) error {
			token, err := tokenV.GetData()
			if err != nil {
				return err
			}
			if token == "" {
				return c.setContentByName(validatorPanel, "Cluster token is required")
			}
			if err := checkToken(token); err != nil {
				return c.setContentByName(validatorPanel, err.Error())
			}
			c.config.Token = token
			if err := closeThisPage(); err != nil {
				return err
			}
			return showNext(c, ntpServersPanel)
		},
		gocui.KeyEsc: func(g *gocui.Gui, _ *gocui.View) error {
			if err := closeThisPage(); err != nil {
				return err
			}
			if c.config.Install.Mode == config.ModeCreate {
				g.Cursor = false
				return showNext(c, vipTextPanel, askVipMethodPanel)
			}
			return showNext(c, serverURLPanel)
		},
	}
	c.AddElement(tokenPanel, tokenV)
	return nil
}

func showNetworkPage(c *Console) error {
	if mgmtNetwork.Method != config.NetworkMethodStatic {
		return showNext(c, askVlanIDPanel, askBondModePanel, askNetworkMethodPanel, askInterfacePanel)
	}
	return showNext(c, askVlanIDPanel, askBondModePanel, askNetworkMethodPanel, addressPanel, addrMaskPanel, gatewayPanel, mtuPanel, askInterfacePanel)
}

func showHostnamePage(c *Console) error {
	setLocation := createVerticalLocatorWithName(c)

	if err := setLocation(hostnamePanel, 3); err != nil {
		return err
	}

	if err := setLocation(hostnameValidatorPanel, 3); err != nil {
		return err
	}
	return showNext(c, hostnamePanel)
}

func addHostnamePanel(c *Console) error {
	hostnameV, err := widgets.NewInput(c.Gui, hostnamePanel, hostNameLabel, false)
	if err != nil {
		return err
	}

	validatorV := widgets.NewPanel(c.Gui, hostnameValidatorPanel)
	validatorV.FgColor = gocui.ColorRed
	validatorV.Focus = false

	maxX, _ := c.Gui.Size()
	validatorV.X1 = maxX / 8 * 6

	updateValidateMessage := func(message string) error {
		return c.setContentByName(hostnameValidatorPanel, message)
	}

	getNextPagePanel := func() []string {
		return []string{dnsServersPanel}
	}

	next := func() error {
		c.CloseElements(hostnamePanel, hostnameValidatorPanel)
		return showNext(c, getNextPagePanel()...)
	}

	prev := func(_ *gocui.Gui, _ *gocui.View) error {
		c.CloseElements(hostnamePanel, hostnameValidatorPanel)
		if c.config.Install.Mode == config.ModeCreate {
			return showClusterNetworkPage(c)
		}
		return showNetworkPage(c)
	}

	validate := func() (string, error) {
		hostname, err := hostnameV.GetData()
		if err != nil {
			return "", err
		}

		if hostname == "" {
			return "Must specify hostname.", nil
		}

		if errs := validation.IsDNS1123Subdomain(hostname); len(errs) > 0 {
			return "Invalid hostname. A lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters, '-' or '.'.", nil
		}
		c.config.Hostname = hostname
		return "", nil
	}

	hostnameV.PreShow = func() error {
		c.Gui.Cursor = true
		// On the first run through the interactive installer, the hostname is
		// not yet set in the harvester config, but we might have been given
		// a new hostname via DHCP...
		checkDHCPHostname(c.config, false)
		hostnameV.Value = c.config.Hostname
		return c.setContentByName(titlePanel, hostnameTitle)
	}
	hostnameV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyArrowUp: prev,
		gocui.KeyEnter: func(_ *gocui.Gui, _ *gocui.View) error {
			message, err := validate()
			if err != nil {
				return err
			}

			if message != "" {
				return updateValidateMessage(message)
			}
			return next()
		},
		gocui.KeyEsc: prev,
	}

	c.AddElement(hostnamePanel, hostnameV)
	c.AddElement(hostnameValidatorPanel, validatorV)
	return nil
}

// ParseMask parses s as an IP mask, returning the result.
// The string s must be in IPv4 dotted decimal form ("255.255.255.0").
// Each octet must be a valid number between 0 and 255, and the bits must be
// continuous (all 1s followed by all 0s when viewed as a 32-bit number).
// If s is not a valid textual representation of an IP mask,
// ParseMask returns an error describing why the parsing failed.
func ParseMask(mask string) (IPMask, error) {
	// Split the mask string by dots
	parts := strings.Split(mask, ".")

	// Validate number of parts
	if len(parts) != 4 {
		return nil, &ParseError{Type: "mask format must be x.x.x.x, instead of", Text: mask}
	}

	result := make(IPMask, 4)

	// Parse and validate each octet
	for i, part := range parts {
		// Convert string to integer
		num, err := strconv.Atoi(part)
		if err != nil {
			return nil, &ParseError{Type: fmt.Sprintf("unknown number in position %d: %s", i+1, part), Text: mask}
		}
		// Validate range (0-255)
		if num < 0 || num > 255 {
			return nil, &ParseError{Type: fmt.Sprintf("number out of range in position %d: %d", i+1, num), Text: mask}
		}

		// #nosec G602
		result[i] = byte(num)
	}

	// Validate mask format (must be continuous 1s followed by continuous 0s)
	if !isValidMaskFormat(result) {
		return nil, &ParseError{Type: "invalid subnet mask: not continuous", Text: mask}
	}

	return result, nil
}

// Helper function to validate mask format
func isValidMaskFormat(mask IPMask) bool {
	valid := false
	outMask := net.IPv4Mask(mask[0], mask[1], mask[2], mask[3])
	if outMask != nil {
		ones, _ := outMask.Size()
		cidrMask := net.CIDRMask(int(ones), 32)
		valid = outMask.String() == cidrMask.String()
	}

	return valid
}

func addNetworkPanel(c *Console) error {
	setLocation := createVerticalLocator(c)

	askInterfaceV, err := widgets.NewDropDown(c.Gui, askInterfacePanel, askInterfaceLabel, getNetworkInterfaceOptions)
	if err != nil {
		return err
	}

	askVlanIDV, err := widgets.NewInput(c.Gui, askVlanIDPanel, askVlanIDLabel, false)
	if err != nil {
		return err
	}

	askBondModeV, err := widgets.NewDropDown(c.Gui, askBondModePanel, askBondModeLabel, getBondModeOptions)
	if err != nil {
		return err
	}

	askNetworkMethodV, err := widgets.NewDropDown(c.Gui, askNetworkMethodPanel, askNetworkMethodLabel, getNetworkMethodOptions)
	if err != nil {
		return err
	}

	addressV, err := widgets.NewInput(c.Gui, addressPanel, addressLabel, false)
	if err != nil {
		return err
	}

	addrMaskV, err := widgets.NewInput(c.Gui, addrMaskPanel, addrMaskLabel, false)
	if err != nil {
		return err
	}

	gatewayV, err := widgets.NewInput(c.Gui, gatewayPanel, gatewayLabel, false)
	if err != nil {
		return err
	}

	mtuV, err := widgets.NewInput(c.Gui, mtuPanel, mtuLabel, false)
	if err != nil {
		return err
	}

	bondNoteV := widgets.NewPanel(c.Gui, bondNotePanel)

	networkValidatorV := widgets.NewPanel(c.Gui, networkValidatorPanel)

	showBondNote := func() error {
		if err := networkValidatorV.Close(); err != nil {
			return err
		}
		if err := bondNoteV.Close(); err != nil {
			return err
		}
		bondNoteV.Focus = false
		bondNoteMsg := bondNote
		// This is just for display purposes on the network screen
		for _, warning := range c.doNetworkSpeedCheck(mgmtNetwork.Interfaces) {
			bondNoteMsg += "\n" + warning
		}
		return c.setContentByName(bondNotePanel, bondNoteMsg)
	}

	updateValidatorMessage := func(msg string) error {
		if err := networkValidatorV.Close(); err != nil {
			return err
		}
		if err := bondNoteV.Close(); err != nil {
			return err
		}
		networkValidatorV.Focus = false
		return c.setContentByName(networkValidatorPanel, msg)
	}

	gotoNextPanel := func(c *Console, name []string, hooks ...func() (string, error)) func(g *gocui.Gui, v *gocui.View) error {
		return func(_ *gocui.Gui, _ *gocui.View) error {
			c.CloseElement(networkValidatorPanel)
			for _, hook := range hooks {
				msg, err := hook()
				if err != nil {
					return err
				}
				if msg != "" {
					return updateValidatorMessage(msg)
				}
			}
			if err := showBondNote(); err != nil {
				return err
			}
			return showNext(c, name...)
		}
	}

	closeThisPage := func() {
		c.CloseElements(
			askInterfacePanel,
			askVlanIDPanel,
			askBondModePanel,
			askNetworkMethodPanel,
			addressPanel,
			addrMaskPanel,
			gatewayPanel,
			mtuPanel,
			networkValidatorPanel,
			bondNotePanel,
		)
	}

	setupNetwork := func() error {
		return applyNetworks(
			mgmtNetwork,
			c.config.Hostname,
		)
	}

	preGotoNextPage := func() (string, error) {
		err := setupNetwork()
		if err != nil {
			return fmt.Sprintf("Configure network failed: %s", err), nil
		}
		logrus.Infof("Network configuration is applied")

		c.config.ManagementInterface = mgmtNetwork

		if mgmtNetwork.Method == config.NetworkMethodDHCP {
			addr, err := getIPThroughDHCP(getManagementInterfaceName(c.config.ManagementInterface))
			if err != nil {
				return fmt.Sprintf("Requesting IP through DHCP failed: %s", err.Error()), nil
			}
			logrus.Infof("DHCP test passed. Got IP: %s", addr)
			userInputData.Address = ""
			mgmtNetwork.IP = ""
			mgmtNetwork.SubnetMask = ""
			mgmtNetwork.Gateway = ""
			mgmtNetwork.MTU = 0
		}

		isDefaultRouteExist, err := checkDefaultRoute()
		if err != nil {
			return fmt.Sprintf("Failed to check default route: %s.", err.Error()), nil
		}
		if !isDefaultRouteExist && mgmtNetwork.Method == config.NetworkMethodDHCP {
			return ErrMsgNoDefaultRoute, nil
		}

		return "", nil
	}

	gotoNextPage := func(fromPanel string) error {
		if err := networkValidatorV.Show(); err != nil {
			return err
		}
		spinner := NewFocusSpinner(c.Gui, networkValidatorPanel, "Applying network configuration...")
		spinner.Start()
		go func(g *gocui.Gui) {
			msg, err := preGotoNextPage()
			if err != nil || msg != "" {
				var isErr bool
				var errMsg string
				if err != nil {
					isErr, errMsg = true, fmt.Sprintf("failed to execute preGotoNextPage hook: %s", err)
				} else {
					isErr, errMsg = true, msg
				}

				spinner.Stop(isErr, errMsg)
				// Go back to the panel that triggered gotoNextPage
				g.Update(func(_ *gocui.Gui) error {
					return showNext(c, fromPanel)
				})
			} else {
				spinner.Stop(false, "")
				g.Update(func(_ *gocui.Gui) error {
					closeThisPage()
					if c.config.Install.Mode == config.ModeCreate {
						return showClusterNetworkPage(c)
					}
					return showHostnamePage(c)
				})
			}
		}(c.Gui)
		return nil
	}

	gotoPrevPage := func(_ *gocui.Gui, _ *gocui.View) error {
		closeThisPage()
		if alreadyInstalled {
			if c.config.Install.Mode == config.ModeJoin {
				return showNext(c, askRolePanel)
			}
			return showNext(c, askCreatePanel)
		}
		return showDiskPage(c)
	}
	// askInterfaceV
	askInterfaceV.PreShow = func() error {
		askInterfaceV.Focus = true
		return c.setContentByName(titlePanel, networkTitle)
	}
	validateInterface := func() (string, error) {
		ifaces := askInterfaceV.GetMultiData()
		if len(ifaces) == 0 {
			return "Must select at least one interface", nil
		}
		interfaces := make([]config.NetworkInterface, 0, len(ifaces))
		for _, iface := range ifaces {
			switch nicState := getNICState(iface); nicState {
			case NICStateNotFound:
				return fmt.Sprintf("NIC %s not found", iface), nil
			case NICStateDown:
				return fmt.Sprintf("NIC %s is down", iface), nil
			case NICStateLowerDown:
				return fmt.Sprintf("NIC %s is down\nNetwork cable isn't plugged in", iface), nil
			}
			tmpInterface := config.NetworkInterface{
				Name: iface,
			}
			if err := tmpInterface.FindNetworkInterfaceHwAddr(); err != nil {
				return "", err
			}
			interfaces = append(interfaces, tmpInterface)
		}
		mgmtNetwork.Interfaces = interfaces
		return "", nil
	}
	interfaceVConfirm := gotoNextPanel(c, []string{askVlanIDPanel}, validateInterface)
	askInterfaceV.SetMulti(true)
	askInterfaceV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyArrowUp:   gotoPrevPage,
		gocui.KeyArrowDown: interfaceVConfirm,
		gocui.KeyEnter:     interfaceVConfirm,
		gocui.KeyEsc:       gotoPrevPage,
	}
	setLocation(askInterfaceV.Panel, 3)
	c.AddElement(askInterfacePanel, askInterfaceV)

	// askVlanIDV
	askVlanIDV.PreShow = func() error {
		c.Gui.Cursor = true
		if mgmtNetwork.VlanID != 0 {
			askVlanIDV.Value = strconv.Itoa(mgmtNetwork.VlanID)
		} else {
			askVlanIDV.Value = ""
		}
		return nil
	}
	validateVlanID := func() (string, error) {
		vlanIDStr, err := askVlanIDV.GetData()
		if err != nil {
			return "", err
		}
		if vlanIDStr == "" {
			mgmtNetwork.VlanID = 0
			return "", nil
		}
		var vlanID int
		vlanID, err = strconv.Atoi(vlanIDStr)
		if err != nil {
			return ErrMsgVLANShouldBeANumberInRange, nil
		}
		// 0 is unset
		if vlanID < 0 || vlanID > 4094 {
			return ErrMsgVLANShouldBeANumberInRange, nil
		}
		mgmtNetwork.VlanID = vlanID
		return "", nil
	}
	askVlanIDVConfirm := gotoNextPanel(c, []string{askBondModePanel}, validateVlanID)
	askVlanIDV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyArrowUp:   gotoNextPanel(c, []string{askInterfacePanel}, validateVlanID),
		gocui.KeyArrowDown: askVlanIDVConfirm,
		gocui.KeyEnter:     askVlanIDVConfirm,
		gocui.KeyEsc:       gotoPrevPage,
	}
	setLocation(askVlanIDV.Panel, 3)
	c.AddElement(askVlanIDPanel, askVlanIDV)

	// askBondModeV
	askBondModeV.PreShow = func() error {
		if mgmtNetwork.BondOptions == nil {
			askBondModeV.Value = config.BondModeActiveBackup
			mgmtNetwork.BondOptions = map[string]string{
				"mode":   config.BondModeActiveBackup,
				"miimon": "100",
			}
		}
		return nil
	}
	askBondModeVConfirm := func(_ *gocui.Gui, _ *gocui.View) error {
		mode, err := askBondModeV.GetData()
		mgmtNetwork.BondOptions = map[string]string{
			"mode":   mode,
			"miimon": "100",
		}
		if err != nil {
			return err
		}
		if err := showBondNote(); err != nil {
			return err
		}
		if mgmtNetwork.Method != config.NetworkMethodStatic {
			return showNext(c, askNetworkMethodPanel)
		}
		return showNext(c, mtuPanel, gatewayPanel, addressPanel, askNetworkMethodPanel)
	}
	askBondModeV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyArrowUp:   gotoNextPanel(c, []string{askVlanIDPanel}),
		gocui.KeyArrowDown: askBondModeVConfirm,
		gocui.KeyEnter:     askBondModeVConfirm,
		gocui.KeyEsc:       gotoPrevPage,
	}
	setLocation(askBondModeV.Panel, 3)
	c.AddElement(askBondModePanel, askBondModeV)

	// askNetworkMethodV
	askNetworkMethodVConfirm := func(_ *gocui.Gui, _ *gocui.View) error {
		selected, err := askNetworkMethodV.GetData()
		if err != nil {
			return err
		}
		mgmtNetwork.Method = selected
		if selected == config.NetworkMethodStatic {
			return showNext(c, mtuPanel, gatewayPanel, addrMaskPanel, addressPanel)
		}

		c.CloseElements(mtuPanel, gatewayPanel, addrMaskPanel, addressPanel)
		return gotoNextPage(askNetworkMethodPanel)
	}
	askNetworkMethodV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyArrowUp:   gotoNextPanel(c, []string{askBondModePanel}),
		gocui.KeyArrowDown: askNetworkMethodVConfirm,
		gocui.KeyEnter:     askNetworkMethodVConfirm,
		gocui.KeyEsc:       gotoPrevPage,
	}
	setLocation(askNetworkMethodV.Panel, 3)
	c.AddElement(askNetworkMethodPanel, askNetworkMethodV)

	// AddressV
	addressV.PreShow = func() error {
		c.Gui.Cursor = true
		addressV.Value = userInputData.Address
		return nil
	}
	validateAddress := func() (string, error) {
		address, err := addressV.GetData()
		if err != nil {
			return "", err
		}
		if err = checkStaticRequiredString("address", address); err != nil {
			return err.Error(), nil
		}
		userInputData.Address = address
		ip, ipNet, err := net.ParseCIDR(address)
		if err != nil {
			// It's not a CIDR address, but it might be a non-CIDR address
			ip = net.ParseIP(address)
			if ip == nil {
				return fmt.Sprintf("%s is not a valid IP address", address), nil
			}
		}
		// At this point, ip is valid (so save it) but ipNet might be nil (non-CIDR address)
		mgmtNetwork.IP = ip.String()
		if ipNet != nil {
			mask := ipNet.Mask
			userInputData.AddrMask = ipNet.Mask.String()
			mgmtNetwork.SubnetMask = fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])
		}
		return "", nil
	}
	addressVConfirm := gotoNextPanel(c, []string{addrMaskPanel}, validateAddress)
	addressV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyArrowUp: gotoNextPanel(c, []string{askNetworkMethodPanel}, func() (string, error) {
			userInputData.Address, err = addressV.GetData()
			return "", err
		}),
		gocui.KeyArrowDown: addressVConfirm,
		gocui.KeyEnter:     addressVConfirm,
		gocui.KeyEsc:       gotoPrevPage,
	}
	setLocation(addressV.Panel, 3)
	c.AddElement(addressPanel, addressV)

	//AddressMaskV
	addrMaskV.PreShow = func() error {
		c.Gui.Cursor = true
		addrMaskV.Value = mgmtNetwork.SubnetMask
		return nil
	}
	validateAddrMask := func() (string, error) {
		addrMask, err := addrMaskV.GetData()
		if err != nil {
			return "", err
		}
		if err = checkStaticRequiredString("mask", addrMask); err != nil {
			return err.Error(), nil
		}
		ipMask, err := ParseMask(addrMask)
		if err != nil {
			return err.Error(), nil
		}
		userInputData.AddrMask = ipMask.String()
		mgmtNetwork.SubnetMask = fmt.Sprintf("%d.%d.%d.%d", ipMask[0], ipMask[1], ipMask[2], ipMask[3])
		return "", nil
	}
	addrMaskVConfirm := gotoNextPanel(c, []string{gatewayPanel}, validateAddrMask)
	addrMaskV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyArrowUp: gotoNextPanel(c, []string{addressPanel}, func() (string, error) {
			userInputData.AddrMask, err = addrMaskV.GetData()
			return "", err
		}),
		gocui.KeyArrowDown: addrMaskVConfirm,
		gocui.KeyEnter:     addrMaskVConfirm,
		gocui.KeyEsc:       gotoPrevPage,
	}
	setLocation(addrMaskV.Panel, 3)
	c.AddElement(addrMaskPanel, addrMaskV)

	// gatewayV
	gatewayV.PreShow = func() error {
		c.Gui.Cursor = true
		gatewayV.Value = mgmtNetwork.Gateway
		return nil
	}
	validateGateway := func() (string, error) {
		gateway, err := gatewayV.GetData()
		if err != nil {
			return "", err
		}
		if err = checkStaticRequiredString("gateway", gateway); err != nil {
			return err.Error(), nil
		}
		if err = checkIP(gateway); err != nil {
			return err.Error(), nil
		}
		mgmtNetwork.Gateway = gateway
		return "", nil
	}
	gatewayVConfirm := gotoNextPanel(c, []string{mtuPanel}, validateGateway)
	gatewayV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyArrowUp: gotoNextPanel(c, []string{addrMaskPanel}, func() (string, error) {
			mgmtNetwork.Gateway, err = gatewayV.GetData()
			return "", err
		}),
		gocui.KeyArrowDown: gatewayVConfirm,
		gocui.KeyEnter:     gatewayVConfirm,
		gocui.KeyEsc:       gotoPrevPage,
	}
	setLocation(gatewayV.Panel, 3)
	c.AddElement(gatewayPanel, gatewayV)

	// mtuV
	mtuV.PreShow = func() error {
		c.Gui.Cursor = true
		if mgmtNetwork.MTU == 0 {
			mtuV.Value = ""
		} else {
			mtuV.Value = strconv.Itoa(mgmtNetwork.MTU)
		}
		return nil
	}
	validateMTU := func() (string, error) {
		var mtu int
		mtuStr, err := mtuV.GetData()
		if err != nil {
			return "", err
		}

		if mtuStr == "" {
			mtu = 0
		} else {
			mtu, err = strconv.Atoi(mtuStr)
			if err != nil {
				return ErrMsgMTUShouldBeANumber, nil
			}
		}

		if err = checkMTU(mtu); err != nil {
			return err.Error(), nil
		}
		mgmtNetwork.MTU = mtu
		return "", nil
	}
	mtuVConfirm := func(_ *gocui.Gui, _ *gocui.View) error {
		msg, err := validateMTU()
		if err != nil {
			return err
		}
		if msg != "" {
			return updateValidatorMessage(msg)
		}

		return gotoNextPage(mtuPanel)
	}
	mtuV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyArrowUp:   gotoNextPanel(c, []string{gatewayPanel}, validateMTU),
		gocui.KeyArrowDown: mtuVConfirm,
		gocui.KeyEnter:     mtuVConfirm,
		gocui.KeyEsc:       gotoPrevPage,
	}
	setLocation(mtuV.Panel, 3)
	c.AddElement(mtuPanel, mtuV)

	// bondNoteV
	bondNoteV.Wrap = true
	setLocation(bondNoteV, 8)
	c.AddElement(bondNotePanel, bondNoteV)

	// networkValidatorV
	networkValidatorV.FgColor = gocui.ColorRed
	networkValidatorV.Wrap = true
	setLocation(networkValidatorV, 4)
	c.AddElement(networkValidatorPanel, networkValidatorV)

	return nil
}

func showClusterNetworkPage(c *Console) error {
	return showNext(
		c,
		clusterServiceCIDRPanel,
		clusterDNSPanel,
		clusterNetworkNotePanel,
		clusterNetworkValidatorPanel,
		clusterPodCIDRPanel)
}

func addClusterNetworkPanel(c *Console) error {
	// define page navigation
	closePage := func() {
		c.CloseElements(
			clusterPodCIDRPanel,
			clusterServiceCIDRPanel,
			clusterDNSPanel,
			clusterNetworkNotePanel,
			clusterNetworkValidatorPanel)
	}

	prevPage := func(_ *gocui.Gui, _ *gocui.View) error {
		closePage()
		return showNetworkPage(c)
	}

	nextPage := func() error {
		closePage()
		return showHostnamePage(c)
	}

	setLocation := createVerticalLocator(c)

	// set up the pod CIDR input panel
	podCIDRInput, err := widgets.NewInput(
		c.Gui,
		clusterPodCIDRPanel,
		clusterPodCIDRLabel,
		false)
	if err != nil {
		return err
	}
	podCIDRInput.PreShow = func() error {
		c.Cursor = true
		podCIDRInput.Value = c.config.ClusterPodCIDR

		if err := c.setContentByName(
			titlePanel,
			clusterNetworkTitle); err != nil {
			return err
		}

		if err := c.setContentByName(
			clusterNetworkNotePanel,
			clusterNetworkNote); err != nil {
			return err
		}

		// reset any previous error in the validator panel before
		// showing the rest of the page
		return c.setContentByName(clusterNetworkValidatorPanel, "")
	}

	// set up the service CIDR input panel
	serviceCIDRInput, err := widgets.NewInput(
		c.Gui,
		clusterServiceCIDRPanel,
		clusterServiceCIDRLabel,
		false)
	if err != nil {
		return err
	}

	serviceCIDRInput.PreShow = func() error {
		c.Cursor = true
		serviceCIDRInput.Value = c.config.ClusterServiceCIDR
		return nil
	}

	// set up the cluster DNS input panel
	dnsInput, err := widgets.NewInput(
		c.Gui,
		clusterDNSPanel,
		clusterDNSLabel,
		false)
	if err != nil {
		return err
	}

	dnsInput.PreShow = func() error {
		c.Cursor = true
		dnsInput.Value = c.config.ClusterDNS
		return nil
	}

	// define inputs validators
	validateCIDR := func(cidr string) error {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			return nil
		}

		_, err := netip.ParsePrefix(cidr)
		return err
	}

	validateDNSIP := func(ip string) error {
		ip = strings.TrimSpace(ip)
		serviceCIDR, err := serviceCIDRInput.GetData()
		if err != nil {
			return err
		}
		if ip == "" && serviceCIDR == "" {
			return nil
		}

		// the DNS IP address must be well-formed and within the
		// service CIDR
		ipAddr, err := netip.ParseAddr(ip)
		if err != nil {
			return fmt.Errorf("invalid cluster DNS IP: %w", err)
		}

		svcNet, err := netip.ParsePrefix(serviceCIDR)
		if err != nil {
			return fmt.Errorf("to override the cluster DNS IP, the service CIDR must be valid: %w", err)
		}

		if !svcNet.Contains(ipAddr) {
			return fmt.Errorf("invalid cluster DNS IP: %s is not in the service CIDR %s", ip, serviceCIDR)
		}

		return nil
	}

	// define input confirm actions
	podCIDRConfirm := func(_ *gocui.Gui, _ *gocui.View) error {
		podCIDR, err := podCIDRInput.GetData()
		if err != nil {
			return err
		}

		if err := validateCIDR(podCIDR); err != nil {
			return c.setContentByName(
				clusterNetworkValidatorPanel,
				fmt.Sprintf("Invalid pod CIDR: %s", err))
		}
		c.config.ClusterPodCIDR = podCIDR

		// reset any previous error in the validator panel before
		// moving to the next panel
		if err := c.setContentByName(clusterNetworkValidatorPanel, ""); err != nil {
			return err
		}
		return showNext(c, clusterServiceCIDRPanel)
	}

	serviceCIDRConfirm := func(_ *gocui.Gui, _ *gocui.View) error {
		serviceCIDR, err := serviceCIDRInput.GetData()
		if err != nil {
			return err
		}

		if err = validateCIDR(serviceCIDR); err != nil {
			return c.setContentByName(
				clusterNetworkValidatorPanel,
				fmt.Sprintf("Invalid service CIDR: %s", err))
		}
		c.config.ClusterServiceCIDR = serviceCIDR

		// reset any previous error in the validator panel before
		// moving to the next panel
		if err = c.setContentByName(clusterNetworkValidatorPanel, ""); err != nil {
			return err
		}
		return showNext(c, clusterDNSPanel)
	}

	dnsConfirm := func(_ *gocui.Gui, _ *gocui.View) error {
		dns, err := dnsInput.GetData()
		if err != nil {
			return err
		}
		if err = validateDNSIP(dns); err != nil {
			return c.setContentByName(clusterNetworkValidatorPanel, err.Error())
		}
		c.config.ClusterDNS = dns

		// reset the validator panel before moving to the next page
		if err = c.setContentByName(clusterNetworkValidatorPanel, ""); err != nil {
			return err
		}
		return nextPage()
	}

	// configure key bindings and element locations
	podCIDRInput.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyEsc:       prevPage,
		gocui.KeyArrowUp:   prevPage,
		gocui.KeyArrowDown: podCIDRConfirm,
		gocui.KeyEnter:     podCIDRConfirm,
	}
	setLocation(podCIDRInput, 3)
	c.AddElement(clusterPodCIDRPanel, podCIDRInput)

	serviceCIDRInput.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyEsc: prevPage,
		gocui.KeyArrowUp: func(_ *gocui.Gui, _ *gocui.View) error {
			return showNext(c, clusterPodCIDRPanel)
		},
		gocui.KeyArrowDown: serviceCIDRConfirm,
		gocui.KeyEnter:     serviceCIDRConfirm,
	}
	setLocation(serviceCIDRInput, 3)
	c.AddElement(clusterServiceCIDRPanel, serviceCIDRInput)

	dnsInput.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyEsc: prevPage,
		gocui.KeyArrowUp: func(_ *gocui.Gui, _ *gocui.View) error {
			return showNext(c, clusterServiceCIDRPanel)
		},
		gocui.KeyArrowDown: dnsConfirm,
		gocui.KeyEnter:     dnsConfirm,
	}
	setLocation(dnsInput, 3)
	c.AddElement(clusterDNSPanel, dnsInput)

	// set up notes panels
	notePanel := widgets.NewPanel(c.Gui, clusterNetworkNotePanel)
	notePanel.Focus = false
	notePanel.Wrap = true
	setLocation(notePanel, 4)
	c.AddElement(clusterNetworkNotePanel, notePanel)

	// set up validator panel for warning and error messages
	validatorPanel := widgets.NewPanel(c.Gui, clusterNetworkValidatorPanel)
	validatorPanel.FgColor = gocui.ColorRed
	validatorPanel.Focus = false
	maxX, _ := c.Gui.Size()
	validatorPanel.X1 = maxX / 8 * 6
	setLocation(validatorPanel, 3)
	c.AddElement(clusterNetworkValidatorPanel, validatorPanel)

	return nil
}

func getBondModeOptions() ([]widgets.Option, error) {
	return []widgets.Option{
		{
			Value: config.BondModeBalanceRR,
			Text:  config.BondModeBalanceRR,
		},
		{
			Value: config.BondModeActiveBackup,
			Text:  config.BondModeActiveBackup,
		},
		{
			Value: config.BondModeBalnaceXOR,
			Text:  config.BondModeBalnaceXOR,
		},
		{
			Value: config.BondModeBroadcast,
			Text:  config.BondModeBroadcast,
		},
		{
			Value: config.BondModeIEEE802_3ad,
			Text:  config.BondModeIEEE802_3ad,
		},
		{
			Value: config.BondModeBalanceTLB,
			Text:  config.BondModeBalanceTLB,
		},
		{
			Value: config.BondModeBalanceALB,
			Text:  config.BondModeBalanceALB,
		},
	}, nil
}

func getNetworkInterfaceOptions() ([]widgets.Option, error) {
	var options = []widgets.Option{}
	nics, err := getNICs()
	if err != nil {
		return nil, err
	}

	for _, nic := range nics {
		name := nic.Attrs().Name
		option := widgets.Option{
			Value: name,
			Text:  fmt.Sprintf("%s(%s, %s)", name, nic.Attrs().HardwareAddr.String(), nic.Attrs().OperState.String()),
		}
		options = append(options, option)
	}
	return options, nil
}

func getNetworkMethodOptions() ([]widgets.Option, error) {
	return []widgets.Option{
		{
			Value: config.NetworkMethodDHCP,
			Text:  networkMethodDHCPText,
		},
		{
			Value: config.NetworkMethodStatic,
			Text:  networkMethodStaticText,
		},
	}, nil
}

func addProxyPanel(c *Console) error {
	proxyV, err := widgets.NewInput(c.Gui, proxyPanel, "Proxy address", false)
	if err != nil {
		return err
	}
	proxyV.PreShow = func() error {
		c.Gui.Cursor = true
		proxyV.Value = os.Getenv("HTTP_PROXY")
		if err := c.setContentByName(titlePanel, "Optional: configure proxy"); err != nil {
			return err
		}
		return c.setContentByName(notePanel, proxyNote)
	}
	proxyV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyEnter: func(_ *gocui.Gui, _ *gocui.View) error {
			proxy, err := proxyV.GetData()
			if err != nil {
				return err
			}
			if proxy != "" {
				if err = os.Setenv("HTTP_PROXY", proxy); err != nil {
					return err
				}
				if err = os.Setenv("HTTPS_PROXY ", proxy); err != nil {
					return err
				}
			} else {
				if err = os.Unsetenv("HTTP_PROXY"); err != nil {
					return err
				}
				if err = os.Unsetenv("HTTPS_PROXY"); err != nil {
					return err
				}
			}
			if err = proxyV.Close(); err != nil {
				return err
			}
			noteV, err := c.GetElement(notePanel)
			if err != nil {
				return err
			}
			if err = noteV.Close(); err != nil {
				return err
			}
			return showNext(c, sshKeyPanel)
		},
		gocui.KeyEsc: func(_ *gocui.Gui, _ *gocui.View) error {
			if err := proxyV.Close(); err != nil {
				return err
			}
			c.CloseElement(notePanel)
			return showNext(c, ntpServersPanel)
		},
	}
	c.AddElement(proxyPanel, proxyV)
	return nil
}

func addCloudInitPanel(c *Console) error {
	cloudInitV, err := widgets.NewInput(c.Gui, cloudInitPanel, "HTTP URL", false)
	if err != nil {
		return err
	}
	cloudInitV.PreShow = func() error {
		c.Gui.Cursor = true
		cloudInitV.Value = c.config.Install.ConfigURL
		return c.setContentByName(titlePanel, "Optional: remote Harvester config")
	}
	gotoNextPage := func() error {
		if err := cloudInitV.Close(); err != nil {
			return err
		}
		return showNext(c, confirmInstallPanel)
	}
	cloudInitV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyEnter: func(_ *gocui.Gui, _ *gocui.View) error {
			configURL, err := cloudInitV.GetData()
			if err != nil {
				return err
			}
			c.config.Install.ConfigURL = configURL
			if configURL != "" {
				asyncTaskV, err := c.GetElement(spinnerPanel)
				if err != nil {
					return err
				}
				if err = asyncTaskV.Close(); err != nil {
					return err
				}
				if err = asyncTaskV.Show(); err != nil {
					return err
				}

				spinner := NewSpinner(c.Gui, spinnerPanel, fmt.Sprintf("Checking %q...", configURL))
				spinner.Start()

				go func(g *gocui.Gui) {
					if _, err = getRemoteConfig(configURL); err != nil {
						spinner.Stop(true, err.Error())
						g.Update(func(_ *gocui.Gui) error {
							return showNext(c, cloudInitPanel)
						})
						return
					}
					spinner.Stop(false, "")
					g.Update(func(_ *gocui.Gui) error {
						return gotoNextPage()
					})
				}(c.Gui)
				return nil
			}
			return gotoNextPage()
		},
		gocui.KeyEsc: func(_ *gocui.Gui, _ *gocui.View) error {
			if err = cloudInitV.Close(); err != nil {
				return err
			}
			return showNext(c, sshKeyPanel)
		},
	}
	cloudInitV.PostClose = func() error {
		asyncTaskV, err := c.GetElement(spinnerPanel)
		if err != nil {
			return err
		}
		return asyncTaskV.Close()
	}
	c.AddElement(cloudInitPanel, cloudInitV)
	return nil
}

func addConfirmInstallPanel(c *Console) error {
	askOptionsFunc := func() ([]widgets.Option, error) {
		return []widgets.Option{
			{
				Value: "yes",
				Text:  "Yes",
			}, {
				Value: "no",
				Text:  "No (Reboot)",
			},
		}, nil
	}
	confirmV, err := widgets.NewSelect(c.Gui, confirmInstallPanel, "", askOptionsFunc)
	if err != nil {
		return err
	}
	confirmV.PreShow = func() error {
		installBytes, err := config.PrintInstall(*c.config)
		if err != nil {
			return err
		}
		options := fmt.Sprintf("install mode: %v\n", c.config.Install.Mode)
		if !installModeOnly {
			options += fmt.Sprintf("install role: %v\n", c.config.Install.Role)
		}
		options += fmt.Sprintf("hostname: %v\n", c.config.OS.Hostname)
		if userInputData.DNSServers != "" {
			options += fmt.Sprintf("dns servers: %v\n", userInputData.DNSServers)
		}
		if userInputData.NTPServers != "" {
			options += fmt.Sprintf("ntp servers: %v\n", userInputData.NTPServers)
		}
		if proxy := os.Getenv("HTTP_PROXY"); proxy != "" {
			options += fmt.Sprintf("proxy address: %v\n", proxy)
		}
		if userInputData.SSHKeyURL != "" {
			options += fmt.Sprintf("ssh key url: %v\n", userInputData.SSHKeyURL)
		}
		if c.config.OS.SSHD.DisablePasswordAuth {
			options += "disable SSH password auth: yes\n"
		}
		options += string(installBytes)
		logrus.Debug("cfm cfg: ", fmt.Sprintf("%+v", c.config.Install))
		if !c.config.Install.Silent {
			if alreadyInstalled {
				confirmV.SetContent(options +
					"\nHarvester is already installed. It will be configured with the above configuration. Continue?\n")
			} else if installModeOnly {
				confirmV.SetContent(options +
					"\nHarvester will be copied to local disk. No configuration will be performed. Continue?\n")
			} else {
				confirmV.SetContent(options +
					"\nYour disk will be formatted and Harvester will be installed with the above configuration. Continue?\n")
			}
		}
		c.Gui.Cursor = false
		return c.setContentByName(titlePanel, "Confirm installation options")
	}
	confirmV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyEnter: func(_ *gocui.Gui, _ *gocui.View) error {
			confirmed, err := confirmV.GetData()
			if err != nil {
				return err
			}
			if confirmed == "no" {
				if err = confirmV.Close(); err != nil {
					return err
				}
				if err = c.setContentByName(titlePanel, ""); err != nil {
					return err
				}
				if err = c.setContentByName(footerPanel, ""); err != nil {
					return err
				}
				go util.SleepAndReboot() //nolint:errcheck
				return c.setContentByName(notePanel, "Installation halted. Rebooting system in 5 seconds")
			}
			if err = confirmV.Close(); err != nil {
				return err
			}
			return showNext(c, installPanel)
		},
		gocui.KeyEsc: func(_ *gocui.Gui, _ *gocui.View) error {
			if err = confirmV.Close(); err != nil {
				return err
			}
			if installModeOnly {
				return showDiskPage(c)
			}
			return showNext(c, cloudInitPanel)
		},
	}
	c.AddElement(confirmInstallPanel, confirmV)
	return nil
}

func addConfirmUpgradePanel(c *Console) error {
	askOptionsFunc := func() ([]widgets.Option, error) {
		return []widgets.Option{
			{
				Value: "yes",
				Text:  "Yes",
			}, {
				Value: "no",
				Text:  "No",
			},
		}, nil
	}
	confirmV, err := widgets.NewSelect(c.Gui, confirmUpgradePanel, "", askOptionsFunc)
	if err != nil {
		return err
	}
	confirmV.PreShow = func() error {
		return c.setContentByName(titlePanel, fmt.Sprintf("Confirm upgrading Harvester to %s?", version.Version))
	}
	confirmV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyEnter: func(_ *gocui.Gui, _ *gocui.View) error {
			confirmed, err := confirmV.GetData()
			if err != nil {
				return err
			}
			if err = confirmV.Close(); err != nil {
				return err
			}
			if confirmed == "no" {
				return showNext(c, askCreatePanel)
			}
			return showNext(c, upgradePanel)
		},
		gocui.KeyEsc: func(_ *gocui.Gui, _ *gocui.View) error {
			if err = confirmV.Close(); err != nil {
				return err
			}
			return showNext(c, askCreatePanel)
		},
	}
	c.AddElement(confirmUpgradePanel, confirmV)
	return nil
}

func addInstallPanel(c *Console) error {
	maxX, maxY := c.Gui.Size()
	installV := widgets.NewPanel(c.Gui, installPanel)
	installV.PreShow = func() error {
		go func() {
			// Legacy BIOS systems are no longer supported
			biosCheck := preflight.BIOSCheck{}
			if msg, _ := biosCheck.Run(); len(msg) > 0 {
				logrus.Error(msg)
				printToPanel(c.Gui, msg, installPanel)
				return
			}

			// in alreadyInstalled mode and auto configuration, the network is not available
			if alreadyInstalled && c.config.Automatic && c.config.ManagementInterface.Method == "dhcp" {
				configureInstallModeDHCP(c)
			}

			// Need to merge remote config first
			logrus.Info("Local config: ", c.config)
			if c.config.Install.ConfigURL != "" {
				printToPanel(c.Gui, fmt.Sprintf("Fetching %s...", c.config.Install.ConfigURL), installPanel)
				remoteConfig, err := retryRemoteConfig(c.config.Install.ConfigURL, c.Gui)
				if err != nil {
					logrus.Error(err)
					printToPanel(c.Gui, err.Error(), installPanel)
					return
				}
				logrus.Info("Remote config: ", remoteConfig)
				if err := c.config.Merge(*remoteConfig); err != nil {
					printToPanel(c.Gui, fmt.Sprintf("fail to merge config: %s", err), installPanel)
					return
				}
				logrus.Info("Local config (merged): ", c.config)
			}

			// case insensitive for network method and vip mode
			c.config.ManagementInterface.Method = strings.ToLower(c.config.ManagementInterface.Method)
			c.config.VipMode = strings.ToLower(c.config.VipMode)

			// lookup MAC Address to populate device names where needed
			// lookup device name to populate MAC Address
			// This needs to happen early, before a possible call to
			// applyNetworks() in the DHCP case.
			for i := range c.config.ManagementInterface.Interfaces {
				if err := c.config.ManagementInterface.Interfaces[i].FindNetworkInterfaceNameAndHwAddr(); err != nil {
					logrus.Error(err)
					printToPanel(c.Gui, err.Error(), installPanel)
					return
				}
			}

			if c.config.Automatic && c.config.Install.ManagementInterface.Method == config.NetworkMethodDHCP {
				// Only need to do this for automatic installs, as manual installs will
				// have already run applyNetworks()
				printToPanel(c.Gui, "Configuring network...", installPanel)
				if err := applyNetworks(c.config.ManagementInterface, c.config.Hostname); err != nil {
					printToPanel(c.Gui, fmt.Sprintf("Can't apply networks: %s", err), installPanel)
					return
				}
			}

			if needToGetVIPFromDHCP(c.config.VipMode, c.config.Vip, c.config.VipHwAddr) {
				vip, err := getVipThroughDHCP(getManagementInterfaceName(c.config.ManagementInterface), "")
				if err != nil {
					printToPanel(c.Gui, fmt.Sprintf("fail to get vip: %s", err), installPanel)
					return
				}
				c.config.Vip = vip.ipv4Addr
				c.config.VipHwAddr = vip.hwAddr
			}

			// If no hostname was provided in the config, this function will
			// default the hostname to either what's supplied by the DHCP sever,
			// or a randomly generated name.
			checkDHCPHostname(c.config, true)

			if c.config.TTY == "" {
				c.config.TTY = getFirstConsoleTTY()
			}
			if c.config.ServerURL != "" {
				formatted, err := getFormattedServerURL(c.config.ServerURL)
				if err != nil {
					printToPanel(c.Gui, fmt.Sprintf("server url invalid: %s", err), installPanel)
					return
				}
				c.config.ServerURL = formatted
			}

			if !alreadyInstalled {
				// Have to handle preflight warnings here because we can't check
				// the NIC speed until we've got the correct set of interfaces.
				preflightWarnings = append(preflightWarnings, c.doNetworkSpeedCheck(c.config.ManagementInterface.Interfaces)...)
				if len(preflightWarnings) > 0 {
					if c.config.SkipChecks || preflightAck {
						// User is happy to skip checks so let installation proceed,
						// but still log the warning messages (this happens for both
						// interactive and automatic/PXE install)
						for _, warning := range preflightWarnings {
							logrus.Warning(warning)
						}
						logrus.Info("Installation will proceed (harvester.install.skipchecks = true)")
					} else {
						// Checks were not explicitly skipped, fail the install
						// (this will happen when PXE booted if checks fail and
						// you don't set harvester.install.skipcheck=true)
						for _, warning := range preflightWarnings {
							logrus.Error(warning)
							printToPanel(c.Gui, warning, installPanel)
						}
						return
					}
				}
			}

			isDefaultRouteExist, err := checkDefaultRoute()
			if err != nil {
				logrus.Error(err)
				printToPanel(c.Gui, "Failed to check default route.", installPanel)
				return
			}
			if !installModeOnly && !isDefaultRouteExist && c.config.Install.ManagementInterface.Method == config.NetworkMethodDHCP {
				logrus.Error(ErrMsgNoDefaultRoute)
				printToPanel(c.Gui, ErrMsgNoDefaultRoute, installPanel)
				return
			}

			// We need ForceGPT because cOS only supports ForceGPT (--force-gpt) flag, not ForceMBR!
			c.config.ForceGPT = !c.config.ForceMBR

			// Clear the DataDisk field if it's identical to the installation disk
			if c.config.DataDisk == c.config.Device {
				c.config.DataDisk = ""
			}

			if err := validateConfig(ConfigValidator{}, c.config); err != nil {
				msg := fmt.Sprintf("Invalid configuration: %s", err)
				logrus.Error(msg)
				printToPanel(c.Gui, msg, installPanel)
				return
			}

			webhooks, err := PrepareWebhooks(c.config.Webhooks, getWebhookContext(c.config))
			if err != nil {
				msg := fmt.Sprintf("Invalid webhook: %s", err)
				logrus.Error(msg)
				printToPanel(c.Gui, msg, installPanel)
			}

			if alreadyInstalled {
				err = configureInstalledNode(c.Gui, c.config, webhooks)
			} else {
				err = doInstall(c.Gui, c.config, webhooks)
			}
			if err != nil {
				msg := fmt.Sprintf("Install failed: %s", err)
				logrus.Error(msg)
				printToPanel(c.Gui, msg, installPanel)
			}
		}()
		return c.setContentByName(footerPanel, "")
	}
	installV.Title = " Installing Harvester "
	installV.SetLocation(maxX/8, maxY/8, maxX/8*7, maxY/8*7)
	installV.Wrap = true
	installV.Autoscroll = true
	c.AddElement(installPanel, installV)
	installV.Frame = true
	return nil
}

func addSpinnerPanel(c *Console) error {
	maxX, maxY := c.Gui.Size()
	asyncTaskV := widgets.NewPanel(c.Gui, spinnerPanel)
	asyncTaskV.SetLocation(maxX/8, maxY/8+7, maxX/8*7, maxY/8*7)
	asyncTaskV.Wrap = true
	c.AddElement(spinnerPanel, asyncTaskV)
	return nil
}

func addUpgradePanel(c *Console) error {
	maxX, maxY := c.Gui.Size()
	upgradeV := widgets.NewPanel(c.Gui, upgradePanel)
	upgradeV.PreShow = func() error {
		go doUpgrade(c.Gui) //nolint:errcheck
		return c.setContentByName(footerPanel, "")
	}
	upgradeV.Title = " Upgrading Harvester "
	upgradeV.SetLocation(maxX/8, maxY/8, maxX/8*7, maxY/8*7)
	c.AddElement(upgradePanel, upgradeV)
	upgradeV.Frame = true
	return nil
}

func addVIPPanel(c *Console) error {
	setLocation := createVerticalLocator(c)

	askVipMethodV, err := widgets.NewDropDown(c.Gui, askVipMethodPanel, askVipMethodLabel, getNetworkMethodOptions)
	if err != nil {
		return err
	}
	hwAddrV, err := widgets.NewInput(c.Gui, vipHwAddrPanel, vipHwAddrLabel, false)
	if err != nil {
		return err
	}
	vipV, err := widgets.NewInput(c.Gui, vipPanel, vipLabel, false)
	if err != nil {
		return err
	}
	hwAddrNoteV := widgets.NewPanel(c.Gui, vipHwAddrNotePanel)
	vipTextV := widgets.NewPanel(c.Gui, vipTextPanel)

	closeThisPage := func() {
		c.CloseElements(
			askVipMethodPanel,
			vipHwAddrPanel,
			vipHwAddrNotePanel,
			vipPanel,
			vipTextPanel)
	}

	gotoPrevPage := func(_ *gocui.Gui, _ *gocui.View) error {
		closeThisPage()
		return showNext(c, dnsServersPanel)
	}
	gotoNextPage := func(_ *gocui.Gui, _ *gocui.View) error {
		closeThisPage()
		return showNext(c, tokenPanel)
	}
	gotoVipPanel := func(g *gocui.Gui, _ *gocui.View) error {
		selected, err := askVipMethodV.GetData()
		if err != nil {
			return err
		}
		hwAddr, err := hwAddrV.GetData()
		if err != nil {
			return err
		}
		if selected == config.NetworkMethodDHCP {
			spinner := NewSpinner(c.Gui, vipTextPanel, "Requesting IP through DHCP...")
			spinner.Start()
			go func(g *gocui.Gui) {
				vip, err := getVipThroughDHCP(getManagementInterfaceName(c.config.ManagementInterface), hwAddr)
				if err != nil {
					spinner.Stop(true, err.Error())
					g.Update(func(_ *gocui.Gui) error {
						return showNext(c, askVipMethodPanel)
					})
					return
				}
				spinner.Stop(false, "")
				c.config.Vip = vip.ipv4Addr
				c.config.VipMode = selected
				c.config.VipHwAddr = vip.hwAddr
				g.Update(func(_ *gocui.Gui) error {
					if err := hwAddrV.SetData(vip.hwAddr); err != nil {
						return err
					}
					return vipV.SetData(vip.ipv4Addr)
				})
			}(c.Gui)
		} else {
			vipTextV.SetContent("")
			g.Update(func(_ *gocui.Gui) error {
				return vipV.SetData("")
			})
			c.config.VipMode = config.NetworkMethodStatic
		}

		return showNext(c, vipPanel)
	}
	gotoVerifyIP := func(g *gocui.Gui, v *gocui.View) error {
		vip, err := vipV.GetData()
		if err != nil {
			return err
		}

		if c.config.VipMode == config.NetworkMethodDHCP {
			if vip != c.config.Vip {
				vipTextV.SetContent("Forbid to modify the VIP obtained through DHCP")
				return nil
			}
			return gotoNextPage(g, v)
		}

		// verify static IP
		if net.ParseIP(vip) == nil {
			vipTextV.SetContent(fmt.Sprintf("Invalid VIP: %s", vip))
			return nil
		}

		if vip != "" && vip == mgmtNetwork.IP {
			vipTextV.SetContent("VIP must not be the same as management NIC's IP")
			return nil
		}

		c.config.Vip = vip
		c.config.VipHwAddr = ""
		// gotoVipPanel is only called in DHCP mode, it is still empty in static mode
		if c.config.VipMode == "" {
			c.config.VipMode = config.NetworkMethodStatic
		}
		return gotoNextPage(g, v)
	}
	gotoAskVipMethodPanel := func(_ *gocui.Gui, _ *gocui.View) error {
		return showNext(c, askVipMethodPanel)
	}
	confirmAskVipMethod := func(_ *gocui.Gui, _ *gocui.View) error {
		method, err := askVipMethodV.GetData()
		if err != nil {
			return err
		}
		if method == config.NetworkMethodDHCP {
			hwAddrNoteV.SetContent("Note: If DHCP MAC/IP address binding is configured on the DHCP server, enter the MAC address to fetch the static VIP. Otherwise, leave it blank.")
			return showNext(c, vipPanel, vipHwAddrNotePanel, vipHwAddrPanel)
		}

		c.CloseElement(vipHwAddrPanel)
		c.CloseElement(vipHwAddrNotePanel)

		return showNext(c, vipPanel)
	}
	gotoVipParentPanel := func(_ *gocui.Gui, _ *gocui.View) error {
		method, err := askVipMethodV.GetData()
		if err != nil {
			return err
		}
		if method == config.NetworkMethodDHCP {
			if err := showNext(c, vipHwAddrPanel); err != nil {
				return err
			}
			return hwAddrV.SetData(c.config.VipHwAddr)
		}
		return showNext(c, askVipMethodPanel)
	}
	askVipMethodV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyArrowDown: confirmAskVipMethod,
		gocui.KeyEnter:     confirmAskVipMethod,
		gocui.KeyEsc:       gotoPrevPage,
	}
	hwAddrV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyArrowUp:   gotoAskVipMethodPanel,
		gocui.KeyArrowDown: gotoVipPanel,
		gocui.KeyEnter:     gotoVipPanel,
		gocui.KeyEsc:       gotoPrevPage,
	}
	vipV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyArrowUp:   gotoVipParentPanel,
		gocui.KeyArrowDown: gotoVerifyIP,
		gocui.KeyEnter:     gotoVerifyIP,
		gocui.KeyEsc:       gotoPrevPage,
	}

	askVipMethodV.PreShow = func() error {
		c.Gui.Cursor = true
		vipTextV.SetContent("")
		return c.setContentByName(titlePanel, vipTitle)
	}

	setLocation(askVipMethodV, 3)
	c.AddElement(askVipMethodPanel, askVipMethodV)

	setLocation(hwAddrV, 3)
	c.AddElement(vipHwAddrPanel, hwAddrV)

	setLocation(vipV, 3)
	c.AddElement(vipPanel, vipV)

	hwAddrNoteV.Focus = false
	hwAddrNoteV.Wrap = true
	setLocation(hwAddrNoteV, 3)
	c.AddElement(vipHwAddrNotePanel, hwAddrNoteV)

	vipTextV.FgColor = gocui.ColorRed
	vipTextV.Focus = false
	vipTextV.Wrap = true
	setLocation(vipTextV, 3)
	c.AddElement(vipTextPanel, vipTextV)

	return nil
}

func addNTPServersPanel(c *Console) error {
	ntpServersV, err := widgets.NewInput(c.Gui, ntpServersPanel, ntpServersLabel, false)
	if err != nil {
		return err
	}

	ntpServersV.PreShow = func() error {
		c.Gui.Cursor = true
		ntpServersV.Value = userInputData.NTPServers
		if err = c.setContentByName(titlePanel, "Configure NTP Servers"); err != nil {
			return err
		}
		return c.setContentByName(notePanel, ntpServersNote)
	}

	closeThisPage := func() error {
		c.CloseElement(notePanel)
		return ntpServersV.Close()
	}
	gotoPrevPage := func(_ *gocui.Gui, _ *gocui.View) error {
		userInputData.HasCheckedNTPServers = false
		if err := closeThisPage(); err != nil {
			return err
		}
		return showNext(c, tokenPanel)
	}
	gotoNextPage := func() error {
		userInputData.HasCheckedNTPServers = false
		if err := closeThisPage(); err != nil {
			return err
		}
		return showNext(c, proxyPanel)
	}
	gotoSpinnerErrorPage := func(g *gocui.Gui, spinner *Spinner, msg string) {
		spinner.Stop(true, msg)
		g.Update(func(_ *gocui.Gui) error {
			return showNext(c, ntpServersPanel)
		})
	}

	ntpServersV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyEnter: func(_ *gocui.Gui, _ *gocui.View) error {
			// get ntp servers input
			ntpServers, err := ntpServersV.GetData()
			if err != nil {
				return err
			}

			// When input servers can't be reached and users don't want to change it, we continue the process.
			if userInputData.NTPServers == ntpServers && userInputData.HasCheckedNTPServers {
				return gotoNextPage()
			}
			// reset HasCheckedNTPServers if users change input
			userInputData.HasCheckedNTPServers = false

			// init asyncTaskV
			asyncTaskV, err := c.GetElement(spinnerPanel)
			if err != nil {
				return err
			}
			if err = asyncTaskV.Close(); err != nil {
				return err
			}

			// focus on task panel to prevent input
			if err = asyncTaskV.Show(); err != nil {
				return err
			}

			spinner := NewSpinner(c.Gui, spinnerPanel, fmt.Sprintf("Checking NTP Server: %q...", ntpServers))
			spinner.Start()

			go func(g *gocui.Gui) {
				if strings.TrimSpace(ntpServers) != ntpServers {
					gotoSpinnerErrorPage(g, spinner, "There is space in input.")
					return
				}

				userInputData.HasCheckedNTPServers = true
				userInputData.NTPServers = ntpServers
				ntpServerList := strings.Split(ntpServers, ",")
				c.config.OS.NTPServers = ntpServerList

				if ntpServers == "" {
					gotoSpinnerErrorPage(g, spinner, "Empty NTP Server is not recommended. Press Enter again to use current configuration anyway.")
					return
				}

				if err = validateNTPServers(ntpServerList); err != nil {
					logrus.Errorf("validate ntp servers: %v", err)
					gotoSpinnerErrorPage(g, spinner, fmt.Sprintf("Failed to reach NTP servers: %v. Press Enter again to use current configuration anyway, or change the value to revalidate.", err))
					return
				}
				if err = enableNTPServers(ntpServerList); err != nil {
					logrus.Errorf("enable ntp servers: %v", err)
					gotoSpinnerErrorPage(g, spinner, fmt.Sprintf("Failed to enable NTP servers: %v. Press Enter to proceed.", err))
					return
				}

				spinner.Stop(false, "")
				g.Update(func(_ *gocui.Gui) error {
					return gotoNextPage()
				})
			}(c.Gui)
			return nil
		},
		gocui.KeyEsc: gotoPrevPage,
	}
	ntpServersV.PostClose = func() error {
		if err := c.setContentByName(notePanel, ""); err != nil {
			return err
		}
		asyncTaskV, err := c.GetElement(spinnerPanel)
		if err != nil {
			return err
		}
		return asyncTaskV.Close()
	}
	c.AddElement(ntpServersPanel, ntpServersV)

	return nil
}

func addDNSServersPanel(c *Console) error {
	dnsServersV, err := widgets.NewInput(c.Gui, dnsServersPanel, dnsServersLabel, false)
	if err != nil {
		return err
	}

	dnsServersV.PreShow = func() error {
		c.Gui.Cursor = true
		dnsServersV.Value = userInputData.DNSServers
		if err = c.setContentByName(titlePanel, "Configure DNS Servers"); err != nil {
			return err
		}
		return c.setContentByName(notePanel, dnsServersNote)
	}

	closeThisPage := func() error {
		c.CloseElement(notePanel)
		return dnsServersV.Close()
	}
	gotoPrevPage := func(_ *gocui.Gui, _ *gocui.View) error {
		if err := closeThisPage(); err != nil {
			return err
		}
		return showHostnamePage(c)
	}
	gotoNextPage := func() error {
		if err := closeThisPage(); err != nil {
			return err
		}
		if c.config.Install.Mode == config.ModeCreate {
			return showNext(c, vipTextPanel, askVipMethodPanel)
		}
		return showNext(c, serverURLPanel)
	}
	gotoSpinnerErrorPage := func(g *gocui.Gui, spinner *Spinner, msg string) {
		spinner.Stop(true, msg)
		g.Update(func(_ *gocui.Gui) error {
			return showNext(c, dnsServersPanel)
		})
	}

	dnsServersV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyEnter: func(_ *gocui.Gui, _ *gocui.View) error {
			// init asyncTaskV
			asyncTaskV, err := c.GetElement(spinnerPanel)
			if err != nil {
				return err
			}
			if err = asyncTaskV.Close(); err != nil {
				return err
			}

			// get dns servers
			dnsServers, err := dnsServersV.GetData()
			if err != nil {
				return err
			}
			dnsServers = strings.TrimSpace(dnsServers)
			userInputData.DNSServers = dnsServers

			// focus on task panel to prevent input
			if err = asyncTaskV.Show(); err != nil {
				return err
			}

			spinner := NewSpinner(c.Gui, spinnerPanel, fmt.Sprintf("Setup DNS Servers: %q...", dnsServers))
			spinner.Start()

			go func(g *gocui.Gui) {
				if mgmtNetwork.Method == config.NetworkMethodStatic && dnsServers == "" {
					gotoSpinnerErrorPage(g, spinner, "DNS servers are required for static IP address")
					return
				}
				if dnsServers != "" {
					// check input syntax
					dnsServerList := strings.Split(dnsServers, ",")
					if err = checkIPList(dnsServerList); err != nil {
						gotoSpinnerErrorPage(g, spinner, err.Error())
						return
					}

					// setup dns
					if err = updateDNSServersAndReloadNetConfig(dnsServerList, mgmtNetwork.VlanID); err != nil {
						gotoSpinnerErrorPage(g, spinner, fmt.Sprintf("Failed to update DNS servers: %v.", err))
						return
					}

					c.config.OS.DNSNameservers = dnsServerList
				}
				spinner.Stop(false, "")
				g.Update(func(_ *gocui.Gui) error {
					return gotoNextPage()
				})
			}(c.Gui)
			return nil
		},
		gocui.KeyEsc: gotoPrevPage,
	}

	dnsServersV.PostClose = func() error {
		if err := c.setContentByName(notePanel, ""); err != nil {
			return err
		}
		asyncTaskV, err := c.GetElement(spinnerPanel)
		if err != nil {
			return err
		}
		return asyncTaskV.Close()
	}
	c.AddElement(dnsServersPanel, dnsServersV)

	return nil
}

func configureInstallModeDHCP(c *Console) {
	netDef := c.config.Install.ManagementInterface
	// copy settings before application //
	mgmtNetwork.Interfaces = netDef.Interfaces
	if netDef.BondOptions == nil {
		mgmtNetwork.BondOptions = map[string]string{
			"mode":   config.BondModeActiveBackup,
			"miimon": "100",
		}
	} else {
		mgmtNetwork.BondOptions = netDef.BondOptions
	}
	mgmtNetwork.Method = netDef.Method
	mgmtNetwork.VlanID = netDef.VlanID

	err := applyNetworks(
		mgmtNetwork,
		c.config.Hostname,
	)
	if err != nil {
		logrus.Error(err)
		printToPanel(c.Gui, fmt.Sprintf("error applying network configuration: %s", err.Error()), installPanel)
	}

	_, err = getIPThroughDHCP(getManagementInterfaceName(c.config.ManagementInterface))
	if err != nil {
		printToPanel(c.Gui, fmt.Sprintf("error getting DHCP address: %s", err.Error()), installPanel)
	}

	// if need vip via dhcp
	if c.config.Install.VipMode == config.NetworkMethodDHCP {
		vip, err := getVipThroughDHCP(getManagementInterfaceName(c.config.ManagementInterface), "")
		if err != nil {
			printToPanel(c.Gui, fmt.Sprintf("fail to get vip: %s", err), installPanel)
			return
		}
		c.config.Vip = vip.ipv4Addr
		c.config.VipHwAddr = vip.hwAddr
	}
}

func checkDHCPHostname(c *config.HarvesterConfig, generate bool) {
	if c.Hostname == "" {
		hostname, err := os.Hostname()
		if err != nil {
			logrus.Errorf("error fetching hostname from underlying OS: %v", err)
		}

		if hostname != defaultHostname && hostname != "" {
			c.Hostname = hostname
		} else {
			if generate {
				c.Hostname = generateHostName()
			}
		}
	}
}

func mergeCloudInit(c *config.HarvesterConfig) error {
	cloudConfig, err := config.ReadUserDataConfig()
	if err != nil {
		return err
	}
	if cloudConfig.Install.Automatic {
		if err = c.Merge(cloudConfig); err != nil {
			return err
		}
		if cloudConfig.OS.Hostname != "" {
			c.OS.Hostname = cloudConfig.OS.Hostname
		}
		if cloudConfig.OS.Password != "" {
			c.OS.Password = cloudConfig.OS.Password
		}
	}

	return nil
}
