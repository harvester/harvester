package upgrade

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
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

type version struct {
	Version              string `json:"version"`
	MinUpgradableVersion string `json:"minUpgradableVersion"`
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
				logrus.Errorf("failed syncing version metadata: %v", err)
			}
		case <-s.ctx.Done():
			ticker.Stop()
			return
		}
	}
}

func (s *versionSyncer) sync() error {
	versionMetadataURL := settings.VersionMetadataURL.Get()
	if versionMetadataURL == "" {
		return nil
	}
	resp, err := s.httpClient.Get(versionMetadataURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("expected 200 response but got %d fetching the version metadata", resp.StatusCode)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	current := settings.ServerVersion.Get()
	versionMetaData, err := getUpgradableVersions(body, current)
	if err != nil {
		return err
	}
	return settings.UpgradableVersions.Set(versionMetaData)
}

func getUpgradableVersions(versionMetaData []byte, currentVersion string) (string, error) {
	var versions []version
	var upgradableVersions []string
	if err := json.Unmarshal(versionMetaData, &versions); err != nil {
		return "", err
	}
	for _, v := range versions {
		if gversion.Compare(currentVersion, v.Version, "<") && gversion.Compare(currentVersion, v.MinUpgradableVersion, ">=") {
			upgradableVersions = append(upgradableVersions, v.Version)
		}
	}
	return strings.Join(upgradableVersions, ","), nil
}
