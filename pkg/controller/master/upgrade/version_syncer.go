package upgrade

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	gversion "github.com/mcuadros/go-version"
	"github.com/rancher/wrangler/pkg/slice"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
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
	namespace  string
	httpClient *http.Client

	versionClient ctlharvesterv1.VersionClient
	versionCache  ctlharvesterv1.VersionCache
}

func newVersionSyncer(ctx context.Context, namespace string, versionClient ctlharvesterv1.VersionClient, versionCache ctlharvesterv1.VersionCache) *versionSyncer {
	return &versionSyncer{
		ctx:       ctx,
		namespace: namespace,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		versionClient: versionClient,
		versionCache:  versionCache,
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
	return s.syncVersions(checkResp, current)
}

func (s *versionSyncer) syncVersions(resp CheckUpgradeResponse, currentVersion string) error {
	if err := s.cleanupVersions(currentVersion); err != nil {
		return err
	}

	for _, v := range resp.Versions {
		newVersion, err := s.getNewVersion(v)
		if err != nil {
			return err
		}

		if !canUpgrade(currentVersion, newVersion) {
			continue
		}

		_, err = s.versionClient.Get(newVersion.Namespace, newVersion.Name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				if _, err := s.versionClient.Create(newVersion); err != nil {
					return err
				}
			} else {
				return err
			}
		}
	}
	return nil
}

// cleanupVersions remove version resources that's can't be upgraded to anymore
func (s *versionSyncer) cleanupVersions(currentVersion string) error {
	versions, err := s.versionClient.List(s.namespace, metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, v := range versions.Items {
		if !canUpgrade(currentVersion, &v) {
			if err := s.versionClient.Delete(v.Namespace, v.Name, &metav1.DeleteOptions{}); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *versionSyncer) getNewVersion(v Version) (*harvesterv1.Version, error) {
	releaseDownloadURL := settings.ReleaseDownloadURL.Get()
	url := fmt.Sprintf("%s/%s/version.yaml", releaseDownloadURL, v.Name)
	resp, err := s.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("failed to download version.yaml from URL: %s", url)
	}

	var newVersion harvesterv1.Version
	if err = yaml.Unmarshal(body, &newVersion); err != nil {
		return nil, err
	}

	newVersion.Namespace = s.namespace
	newVersion.Spec.ReleaseDate = v.ReleaseDate
	newVersion.Spec.MinUpgradableVersion = v.MinUpgradableVersion
	newVersion.Spec.Tags = v.Tags
	return &newVersion, nil
}

func canUpgrade(currentVersion string, newVersion *harvesterv1.Version) bool {
	switch {
	case newVersion.Spec.ISOURL == "" || newVersion.Spec.ISOChecksum == "":
		return false
	case slice.ContainsString(newVersion.Spec.Tags, "dev"):
		return true
	case gversion.Compare(currentVersion, newVersion.Name, "<") && (newVersion.Spec.MinUpgradableVersion == "" || gversion.Compare(currentVersion, newVersion.Spec.MinUpgradableVersion, ">=")):
		return true
	default:
		return false
	}
}
