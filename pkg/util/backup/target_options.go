package backup

import (
	"encoding/json"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

// CheckThirdPartyOptions rejects connection settings unsupported by Restic and
// Kopia. Read global settings live so a recent change cannot bypass admission.
func (c TargetClient) CheckThirdPartyOptions(target *settings.BackupTarget) error {
	if strings.TrimSpace(target.Cert) != "" {
		return fmt.Errorf("backup-target.cert is not supported by Restic and Kopia")
	}
	if target.VirtualHostedStyle {
		return fmt.Errorf("backup-target.virtualHostedStyle=true is not supported by Restic and Kopia")
	}
	ca, err := c.readSetting(settings.AdditionalCA)
	if err != nil {
		return err
	}
	if strings.TrimSpace(ca) != "" {
		return fmt.Errorf("additional-ca is not supported by Restic and Kopia")
	}
	return c.checkProxy()
}

func (c TargetClient) checkProxy() error {
	value, err := c.readSetting(settings.HTTPProxy)
	if err != nil {
		return err
	}
	var proxy util.HTTPProxyConfig
	if err := json.Unmarshal([]byte(value), &proxy); err != nil {
		return fmt.Errorf("cannot validate Restic and Kopia connection options: invalid http-proxy setting")
	}
	// noProxy alone does not configure a proxy connection.
	if strings.TrimSpace(proxy.HTTPProxy) != "" || strings.TrimSpace(proxy.HTTPSProxy) != "" {
		return fmt.Errorf("httpProxy and httpsProxy in the http-proxy setting are not supported by Restic and Kopia")
	}
	return nil
}

func (c TargetClient) readSetting(config settings.Setting) (string, error) {
	if c.Settings == nil {
		return "", fmt.Errorf("backup target client is unavailable")
	}
	setting, err := c.Settings.Get(config.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return config.Default, nil
	}
	if err != nil {
		return "", err
	}
	if value := setting.EffectiveValue(); value != "" {
		return value, nil
	}
	return config.Default, nil
}
