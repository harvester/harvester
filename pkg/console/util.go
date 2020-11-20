package console

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	cfg "github.com/rancher/harvester/pkg/config"

	"github.com/ghodss/yaml"
	"github.com/jroimartin/gocui"
	"github.com/rancher/k3os/pkg/config"
	"k8s.io/apimachinery/pkg/util/rand"
)

func getSSHKeysFromURL(url string) ([]string, error) {
	client := http.Client{
		Timeout: 15 * time.Second,
	}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	body := strings.TrimSuffix(string(b), "\n")
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return nil, fmt.Errorf("got unexpected status code: %d, body: %s", resp.StatusCode, body)
	}
	return strings.Split(body, "\n"), nil
}

func getFormattedServerURL(addr string) string {
	if !strings.HasPrefix(addr, "https://") {
		addr = "https://" + addr
	}
	if !strings.HasSuffix(addr, ":6443") {
		addr = addr + ":6443"
	}
	return addr
}

func showNext(c *Console, names ...string) error {
	for _, name := range names {
		v, err := c.GetElement(name)
		if err != nil {
			return err
		}
		if err := v.Show(); err != nil {
			return err
		}
	}

	validatorV, err := c.GetElement(validatorPanel)
	if err != nil {
		return err
	}
	if err := validatorV.Close(); err != nil {
		return err
	}
	return nil
}

func customizeConfig() {
	//common configs for both server and agent
	cfg.Config.K3OS.DNSNameservers = []string{"8.8.8.8"}
	cfg.Config.K3OS.NTPServers = []string{"ntp.ubuntu.com"}
	cfg.Config.K3OS.Modules = []string{"kvm", "vhost_net"}
	cfg.Config.Hostname = "harvester-" + rand.String(5)

	if installMode == modeJoin && nodeRole == nodeRoleCompute {
		return
	}

	harvesterChartValues["minio.persistence.size"] = "100Gi"
	harvesterChartValues["containers.apiserver.image.imagePullPolicy"] = "IfNotPresent"
	harvesterChartValues["harvester-network-controller.image.pullPolicy"] = "IfNotPresent"
	harvesterChartValues["service.harvester.type"] = "LoadBalancer"
	harvesterChartValues["containers.apiserver.authMode"] = "localUser"

	cfg.Config.WriteFiles = []config.File{
		{
			Owner:              "root",
			Path:               "/var/lib/rancher/k3s/server/manifests/harvester.yaml",
			RawFilePermissions: "0600",
			Content:            getHarvesterManifestContent(harvesterChartValues),
		},
	}
	cfg.Config.K3OS.K3sArgs = []string{
		"server",
		"--disable",
		"local-storage",
		"--flannel-backend",
		"none",
	}

	if installMode == modeCreate {
		cfg.Config.K3OS.K3sArgs = append(cfg.Config.K3OS.K3sArgs,
			"--node-label",
			"svccontroller.k3s.cattle.io/enablelb=true",
		)
	}
}

func doInstall(g *gocui.Gui) error {
	var (
		err      error
		tempFile *os.File
	)
	if cfg.Config.K3OS.Install.ConfigURL == "" {
		tempFile, err = ioutil.TempFile("/tmp", "k3os.XXXXXXXX")
		if err != nil {
			return err
		}
		defer tempFile.Close()

		cfg.Config.K3OS.Install.ConfigURL = tempFile.Name()
	}
	ev, err := config.ToEnv(cfg.Config)
	if err != nil {
		return err
	}
	if tempFile != nil {
		cfg.Config.K3OS.Install = nil
		bytes, err := yaml.Marshal(&cfg.Config)
		if err != nil {
			return err
		}
		if _, err := tempFile.Write(bytes); err != nil {
			return err
		}
		if err := tempFile.Close(); err != nil {
			return err
		}
		defer os.Remove(tempFile.Name())
	}
	cmd := exec.Command("/usr/libexec/k3os/install")
	cmd.Env = append(os.Environ(), ev...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		m := scanner.Text()
		g.Update(func(g *gocui.Gui) error {
			v, err := g.View(installPanel)
			if err != nil {
				return err
			}
			fmt.Fprintln(v, m)

			lines := len(v.BufferLines())
			_, sy := v.Size()
			if lines > sy {
				ox, oy := v.Origin()
				v.SetOrigin(ox, oy+1)
			}
			return nil
		})
	}
	scanner = bufio.NewScanner(stderr)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		m := scanner.Text()
		g.Update(func(g *gocui.Gui) error {
			v, err := g.View(installPanel)
			if err != nil {
				return err
			}
			fmt.Fprintln(v, m)

			lines := len(v.BufferLines())
			_, sy := v.Size()
			if lines > sy {
				ox, oy := v.Origin()
				v.SetOrigin(ox, oy+1)
			}
			return nil
		})
	}
	return nil
}

func getHarvesterManifestContent(values map[string]string) string {
	base := `apiVersion: v1
kind: Namespace
metadata:
  name: harvester-system
---
apiVersion: helm.cattle.io/v1
kind: HelmChart
metadata:
  name: harvester
  namespace: kube-system
spec:
  chart: https://%{KUBERNETES_API}%/static/charts/harvester-0.1.0.tgz
  targetNamespace: harvester-system
  set:
`
	var buffer = bytes.Buffer{}
	buffer.WriteString(base)
	for k, v := range values {
		buffer.WriteString(fmt.Sprintf("    %s: %q\n", k, v))
	}
	return buffer.String()
}
