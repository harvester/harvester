package upgrade

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	gversion "github.com/mcuadros/go-version"
	"github.com/sirupsen/logrus"

	"github.com/harvester/harvester/pkg/settings"
)

const (
	syncInterval = time.Hour
)

type CheckUpgradeRequest struct {
	HarvesterVersion string `json:"harvesterVersion"`
}

type CheckUpgradeResponse struct {
	Versions []Version `json:"versions"`
}

type Version struct {
	Name                 string   `json:"name"` // must be in semantic versioning
	ReleaseDate          string   `json:"releaseDate"`
	MinUpgradableVersion string   `json:"minUpgradableVersion,omitempty"`
	Tags                 []string `json:"tags"`
}

type versionSyncer struct {
	ctx        context.Context
	httpClient *http.Client
}

func newVersionSyncer(ctx context.Context) *versionSyncer {
	return &versionSyncer{
		ctx: ctx,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (s *versionSyncer) start() {
	ticker := time.NewTicker(syncInterval)
	for {
		select {
		case <-ticker.C:
			if err := s.sync(); err != nil {
				logrus.Warnf("failed syncing upgrade versions: %v", err)
			}
		case <-s.ctx.Done():
			ticker.Stop()
			return
		}
	}
}

func (s *versionSyncer) sync() error {
	upgradeCheckerEnabled := settings.UpgradeCheckerEnabled.Get()
	upgradeCheckerURL := settings.UpgradeCheckerURL.Get()
	if upgradeCheckerEnabled != "true" || upgradeCheckerURL == "" {
		return nil
	}
	req := &CheckUpgradeRequest{
		HarvesterVersion: settings.ServerVersion.Get(),
	}
	var requestBody bytes.Buffer
	if err := json.NewEncoder(&requestBody).Encode(req); err != nil {
		return err
	}
	resp, err := s.httpClient.Post(upgradeCheckerURL, "application/json", &requestBody)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("expected 200 response but got %d checking upgrades", resp.StatusCode)
	}

	var checkResp CheckUpgradeResponse
	if err := json.NewDecoder(resp.Body).Decode(&checkResp); err != nil {
		return err
	}

	current := settings.ServerVersion.Get()
	versions, err := getUpgradableVersions(checkResp, current)
	if err != nil {
		return err
	}
	return settings.UpgradableVersions.Set(versions)
}

func getUpgradableVersions(resp CheckUpgradeResponse, currentVersion string) (string, error) {
	var upgradableVersions []string
	for _, v := range resp.Versions {
		if gversion.Compare(currentVersion, v.Name, "<") && gversion.Compare(currentVersion, v.MinUpgradableVersion, ">=") {
			upgradableVersions = append(upgradableVersions, v.Name)
		}
	}
	return strings.Join(upgradableVersions, ","), nil
}
