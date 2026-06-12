package console

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/jroimartin/gocui"
	"github.com/sirupsen/logrus"

	"github.com/harvester/harvester/pkg/installer/config"
	"github.com/harvester/harvester/pkg/installer/util"
	"github.com/harvester/harvester/pkg/installer/version"
	"github.com/harvester/harvester/pkg/installer/widgets"
)

const (
	colorBlack = iota
	colorRed
	colorGreen
	colorYellow
	colorBlue

	statusReady         = "Ready"
	statusNotReady      = "NotReady"
	statusSettingUpNode = "Setting up node"
	statusSettingUpHarv = "Setting up Harvester"

	defaultHarvesterConfig = "/oem/harvester.config"
	defaultCustomConfig    = "/oem/99_custom.yaml"

	logo string = `
██╗░░██╗░█████╗░██████╗░██╗░░░██╗███████╗░██████╗████████╗███████╗██████╗░
██║░░██║██╔══██╗██╔══██╗██║░░░██║██╔════╝██╔════╝╚══██╔══╝██╔════╝██╔══██╗
███████║███████║█████╔╝╚██╗░░██╔╝█████╗░░╚█████╗░░░░██║░░░█████╗░░██████╔╝
██╔══██║██╔══██║██╔══██╗░╚████╔╝░██╔══╝░░░╚═══██╗░░░██║░░░██╔══╝░░██╔══██╗
██║░░██║██║░░██║██║░░██║░░╚██╔╝░░███████╗██████╔╝░░░██║░░░███████╗██║░░██║
╚═╝░░╚═╝╚═╝░░╚═╝╚═╝░░╚═╝░░░╚═╝░░░╚══════╝╚═════╝░░░░╚═╝░░░╚══════╝╚═╝░░╚═╝`
)

type state struct {
	installed     bool
	firstHost     bool
	managementURL string
}

var (
	current state
)

func (c *Console) layoutDashboard(g *gocui.Gui) error {
	once.Do(func() {
		if err := initState(); err != nil {
			logrus.Error(err)
		}
		if err := g.SetKeybinding("", gocui.KeyF12, gocui.ModNone, toShell); err != nil {
			logrus.Error(err)
		}
		logrus.Infof("state: %+v", current)
	})

	if err := clusterPanel(g); err != nil {
		return err
	}

	if err := nodePanel(g); err != nil {
		return err
	}

	if err := footer(g); err != nil {
		return err
	}

	if err := logoPanel(g); err != nil {
		return err
	}
	return nil
}

func clusterPanel(g *gocui.Gui) error {
	maxX, _ := g.Size()
	if v, err := g.SetView("clusterPanel", maxX/2-40, 10, maxX/2+35, 15); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Title = " Harvester Cluster "
	}
	if v, err := g.SetView("managementUrl", maxX/2-39, 10, maxX/2+34, 13); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Frame = false
		v.Wrap = true
		if _, err = fmt.Fprintln(v, "* Management URL:\n  loading..."); err != nil {
			return err
		}
		go syncManagementURL(context.Background(), g)
	}
	if v, err := g.SetView("clusterStatus", maxX/2-39, 13, maxX/2+34, 15); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Frame = false
		v.Wrap = true
		if _, err = fmt.Fprintln(v, "* Status: loading..."); err != nil {
			return err
		}
		go syncHarvesterStatus(context.Background(), g)
	}
	return nil
}

func nodePanel(g *gocui.Gui) error {
	maxX, _ := g.Size()
	if v, err := g.SetView("nodePanel", maxX/2-40, 16, maxX/2+35, 21); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Title = " Node "
	}
	if v, err := g.SetView("nodeInfo", maxX/2-39, 16, maxX/2+34, 19); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Frame = false
		v.Wrap = true
		if _, err = fmt.Fprintln(v, "* Hostname: loading...\n* IP Address: loading..."); err != nil {
			return err
		}
		go syncNodeInfo(context.Background(), g)
	}
	if v, err := g.SetView("nodeStatus", maxX/2-39, 19, maxX/2+34, 21); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Frame = false
		v.Wrap = true
		if _, err = fmt.Fprintln(v, "* Status: loading..."); err != nil {
			return err
		}
		go syncNodeStatus(context.Background(), g)
	}
	return nil
}

func footer(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	if v, err := g.SetView("footer", 0, maxY-2, maxX, maxY); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Frame = false
		if _, err = fmt.Fprintf(v, "<Use F12 to switch between Harvester console and Shell>"); err != nil {
			return err
		}
	}
	return nil
}

func logoPanel(g *gocui.Gui) error {
	maxX, _ := g.Size()
	if v, err := g.SetView("logo", maxX/2-40, 1, maxX/2+40, 9); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Frame = false
		if _, err = fmt.Fprint(v, logo); err != nil {
			return err
		}
		versionStr := "version: " + version.HarvesterVersion
		logoLength := 74
		nSpace := logoLength - len(versionStr)
		if _, err = fmt.Fprintf(v, "\n%*s", nSpace, ""); err != nil {
			return err
		}
		if _, err = fmt.Fprintf(v, "%s", versionStr); err != nil {
			return err
		}
	}
	return nil
}

func toShell(g *gocui.Gui, _ *gocui.View) error {
	g.Cursor = true
	maxX, _ := g.Size()
	adminPasswordFrameV := widgets.NewPanel(g, "adminPasswordFrame")
	adminPasswordFrameV.Frame = true
	adminPasswordFrameV.SetLocation(maxX/2-35, 10, maxX/2+35, 17)
	if err := adminPasswordFrameV.Show(); err != nil {
		return err
	}
	adminPasswordV, err := widgets.NewInput(g, "adminPassword", "Input password: ", true)
	if err != nil {
		return err
	}
	adminPasswordV.SetLocation(maxX/2-30, 12, maxX/2+30, 14)
	validatorV := widgets.NewPanel(g, validatorPanel)
	validatorV.SetLocation(maxX/2-30, 14, maxX/2+30, 16)
	validatorV.FgColor = gocui.ColorRed
	validatorV.Focus = false

	adminPasswordV.KeyBindings = map[gocui.Key]func(*gocui.Gui, *gocui.View) error{
		gocui.KeyEnter: func(_ *gocui.Gui, _ *gocui.View) error {
			passwd, err := adminPasswordV.GetData()
			if err != nil {
				return err
			}
			if validateAdminPassword(passwd) {
				return gocui.ErrQuit
			}
			if err := validatorV.Show(); err != nil {
				return err
			}
			validatorV.SetContent("Invalid credential or password hash algorithm not supported.")
			return nil
		},
		gocui.KeyEsc: func(g *gocui.Gui, _ *gocui.View) error {
			g.Cursor = false
			if err := adminPasswordFrameV.Close(); err != nil {
				return err
			}
			if err := adminPasswordV.Close(); err != nil {
				return err
			}
			return validatorV.Close()
		},
	}
	return adminPasswordV.Show()
}

func validateAdminPassword(passwd string) bool {
	file, err := os.Open("/etc/shadow")
	if err != nil {
		return false
	}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "rancher:") {
			return util.CompareByShadow(passwd, line)
		}
	}
	return false
}

func initState() error {
	envFile := config.RancherdConfigFile
	if _, err := os.Stat(envFile); os.IsNotExist(err) {
		return err
	}
	content, err := os.ReadFile(envFile) //nolint:gosec
	if err != nil {
		return err
	}
	serverURL, err := getServerURLFromRancherdConfig(content)
	if err != nil {
		return err
	}

	if serverURL != "" {
		return os.Setenv("KUBECONFIG", "/var/lib/rancher/rke2/agent/kubelet.kubeconfig")
	} else {
		current.firstHost = true
	}

	return nil
}

func syncManagementURL(ctx context.Context, g *gocui.Gui) {
	// sync url at the beginning
	doSyncManagementURL(g)

	syncDuration := 30 * time.Second
	ticker := time.NewTicker(syncDuration)
	go func() {
		<-ctx.Done()
		ticker.Stop()
	}()
	for range ticker.C {
		doSyncManagementURL(g)
	}
}

func doSyncManagementURL(g *gocui.Gui) {
	managementURL := "Unavailable"
	managementIP := getVIP()
	if managementIP != "" {
		managementURL = fmt.Sprintf("https://%s", managementIP)
		current.managementURL = managementURL
	}

	g.Update(func(g *gocui.Gui) error {
		v, err := g.View("managementUrl")
		if err != nil {
			return err
		}
		v.Clear()
		_, err = fmt.Fprintf(v, "* Management URL:\n  %s", managementURL)
		return err
	})
}

func getVIP() string {
	var cmd string
	if current.firstHost {
		cmd = `kubectl get configmap -n harvester-system vip -o jsonpath='{.data.ip}'`
	} else {
		cmd = `kubectl get svc -n kube-system ingress-expose -o jsonpath='{.status.loadBalancer.ingress[*].ip}'`
	}

	out, err := exec.Command("/bin/sh", "-c", cmd).Output()
	outStr := string(out)
	if err != nil {
		logrus.Errorf(err.Error(), outStr)
		return ""
	}

	return outStr
}

func syncNodeInfo(ctx context.Context, g *gocui.Gui) {
	// sync info at the beginning
	doSyncNodeInfo(g)

	syncDuration := 30 * time.Second
	ticker := time.NewTicker(syncDuration)
	go func() {
		<-ctx.Done()
		ticker.Stop()
	}()
	for range ticker.C {
		doSyncNodeInfo(g)
	}
}

func doSyncNodeInfo(g *gocui.Gui) {
	nodeIP := getNodeInfo()
	g.Update(func(g *gocui.Gui) error {
		v, err := g.View("nodeInfo")
		if err != nil {
			return err
		}
		v.Clear()
		_, err = fmt.Fprintf(v, "%s", nodeIP)
		return err
	})
}

func getNodeInfo() string {
	var (
		cmd      string
		address  string
		hostname string
		out      []byte
		err      error
		device   string
	)

	// find node hostname
	cmd = `hostname | tr -d '\r\n'`
	out, err = exec.Command("/bin/sh", "-c", cmd).Output()
	hostname = string(out)
	if err != nil || hostname == "" {
		logrus.Warnf("node didn't have a hostname")
		hostname = ""
	}

	// find the IP from default route
	cmd = `ip -4 -json route show default | jq -e -j '.[0]["dev"]'`
	out, err = exec.Command("/bin/sh", "-c", cmd).Output()
	device = string(out)
	if err != nil || device == "" {
		logrus.Infof("default gateway is not existing. Fallback to harvester-mgmt")
		// find the IP from harvester-mgmt
		device = "harvester-mgmt"
	}

	// get device primary/first IPv4 address
	cmd = fmt.Sprintf(`ip -4 -json address show dev %s | jq -e -j '.[0]["addr_info"][0]["local"]'`, device)
	out, err = exec.Command("/bin/sh", "-c", cmd).Output()
	address = string(out)
	if err != nil || address == "" {
		logrus.Warnf("Device %s didn't have IP address", device)
		address = ""
	}

	return fmt.Sprintf("* Hostname: %s\n* IP Address: %s", hostname, address)
}

func syncHarvesterStatus(ctx context.Context, g *gocui.Gui) {
	// sync status at the beginning
	doSyncHarvesterStatus(g)

	syncDuration := 30 * time.Second
	ticker := time.NewTicker(syncDuration)
	go func() {
		<-ctx.Done()
		ticker.Stop()
	}()
	for range ticker.C {
		doSyncHarvesterStatus(g)
	}
}

func doSyncHarvesterStatus(g *gocui.Gui) {
	status := getHarvesterStatus()
	g.Update(func(g *gocui.Gui) error {
		v, err := g.View("clusterStatus")
		if err != nil {
			return err
		}
		v.Clear()
		_, err = fmt.Fprintf(v, "* Status: %s", status)
		return err
	})
}

func syncNodeStatus(ctx context.Context, g *gocui.Gui) {
	// sync status at the beginning
	doSyncNodeStatus(g)

	syncDuration := 30 * time.Second
	ticker := time.NewTicker(syncDuration)
	go func() {
		<-ctx.Done()
		ticker.Stop()
	}()
	for range ticker.C {
		doSyncNodeStatus(g)
	}
}

func doSyncNodeStatus(g *gocui.Gui) {
	status := getNodeStatus()
	g.Update(func(g *gocui.Gui) error {
		v, err := g.View("nodeStatus")
		if err != nil {
			return err
		}
		v.Clear()
		_, err = fmt.Fprintf(v, "* Status: %s", status)
		return err
	})
}

func k8sIsReady() bool {
	cmd := exec.Command("/bin/sh", "-c", `kubectl get no -o jsonpath='{.items[*].metadata.name}'`)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		logrus.Error(err, string(output))
		return false
	}
	if string(output) == "" {
		//no node is added
		return false
	}
	return true
}

func chartIsInstalled() bool {
	cmd := exec.Command("/bin/sh", "-c", `kubectl -n fleet-local get ManagedChart harvester -o jsonpath='{.status.conditions}' | jq 'map(select(.type == "Ready" and .status == "True")) | length'`)
	cmd.Env = os.Environ()
	output, err := cmd.Output()
	outStr := string(output)
	if err != nil {
		logrus.Error(err, outStr)
		return false
	}
	if len(outStr) == 0 {
		return false
	}
	processed, err := strconv.Atoi(strings.Trim(outStr, "\n"))
	if err != nil {
		logrus.Error(err, outStr)
		return false
	}

	return processed >= 1
}

func isAPIReady(managementURL, path string) bool {
	if !strings.HasPrefix(current.managementURL, "https://") {
		return false
	}
	command := fmt.Sprintf(`curl -fk %s%s`, managementURL, path)
	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.Env = os.Environ()
	_, err := cmd.CombinedOutput()
	return err == nil
}

func nodeIsPresent() bool {
	hostname, err := os.Hostname()
	if err != nil {
		logrus.Errorf("failed to get hostname: %v", err)
		return false
	}

	kcmd := fmt.Sprintf("kubectl get no %s", hostname)
	cmd := exec.Command("/bin/sh", "-c", kcmd)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		logrus.Error(err, string(output))
		return false
	}

	return true
}

func getHarvesterStatus() string {
	if current.firstHost && !current.installed {
		if !k8sIsReady() || !chartIsInstalled() {
			return statusSettingUpHarv
		}
		current.installed = true
	}

	if !nodeIsPresent() {
		return wrapColor(statusNotReady, colorYellow)
	}

	harvesterAPIReady := isAPIReady(current.managementURL, "/v1/harvester/cluster/local")
	if harvesterAPIReady {
		return wrapColor(statusReady, colorGreen)
	}
	return wrapColor(statusNotReady, colorYellow)
}

func getNodeStatus() string {
	if current.firstHost && !current.installed {
		if !k8sIsReady() || !chartIsInstalled() {
			return statusSettingUpNode
		}
		current.installed = true
	}

	if !nodeIsPresent() {
		return wrapColor(statusNotReady, colorYellow)
	}

	return wrapColor(statusReady, colorGreen)
}

func wrapColor(s string, color int) string {
	return fmt.Sprintf("\033[3%d;7m%s\033[0m", color, s)
}

func (c *Console) getHarvesterConfig() error {
	content, err := os.ReadFile(defaultHarvesterConfig)
	if err != nil {
		if os.IsNotExist(err) {
			logrus.Infof("no existing harvester config detected in %s", defaultHarvesterConfig)
			return nil
		}
		return fmt.Errorf("unable to read default harvester.config file %s: %v", defaultHarvesterConfig, err)
	}

	/*
		Previously, we used yaml.Unmarshal to parse the Harvester configuration.
		We have now switched to using LoadHarvesterConfig for parsing.
		However, there is an inconsistency issue with struct tags when using LoadHarvesterConfig.

		For example:

		In /oem/harvester.config:
		```
		os:
		  externalstorage:
		    enabled: true
		kubeovnoperatorchartversion: 1.13.13
		```

		With yaml.Unmarshal:
		- the field `ExternalStorage.Enabled` resolves to true
		- the field `KubeovnOperatorChartVersion` resolves to "1.13.13"

		With LoadHarvesterConfig:
		- the field `ExternalStorage.Enabled` resolves to false
		- the field `KubeovnOperatorChartVersion` resolves to ""

		LoadHarvesterConfig expects field names like "kubeovnChartVersion" and "externalStorageConfig".
		This case occurs because the yaml and json tags are inconsistent in the HarvesterConfig struct.
		However, we want the configuration to be loaded in a consistent way, so we choose LoadHarvesterConfig here.
		Additionally, this code is only used in the Harvester console and should not affect other components.

		Reference: https://github.com/harvester/harvester-installer/pull/1160
	*/

	conf, err := config.LoadHarvesterConfig(content)
	if err != nil {
		return fmt.Errorf("failed to load harvester config from %s: %v", defaultHarvesterConfig, err)
	}
	c.config = conf

	return nil
}
