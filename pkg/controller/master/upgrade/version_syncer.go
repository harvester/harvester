package upgrade

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	gversion "github.com/mcuadros/go-version"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/v3/pkg/slice"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/settings"
	"github.com/harvester/harvester/pkg/util"
)

const (
	syncInterval = time.Hour

	extraInfoNodeCount  = "nodeCount"
	extraInfoCPUCount   = "cpuCount"
	extraInfoMemorySize = "memorySize"
	extraInfoClusterUID = "clusterUID"
)

type CheckUpgradeRequest struct {
	AppVersion string            `json:"appVersion"`
	ExtraInfo  map[string]string `json:"extraInfo"`
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

	versionClient   ctlharvesterv1.VersionClient
	nodeClient      ctlcorev1.NodeClient
	namespaceClient ctlcorev1.NamespaceClient
}

func newVersionSyncer(ctx context.Context, namespace string, versions ctlharvesterv1.VersionController, nodes ctlcorev1.NodeController, namespaces ctlcorev1.NamespaceController) *versionSyncer {
	return &versionSyncer{
		ctx:       ctx,
		namespace: namespace,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		versionClient:   versions,
		nodeClient:      nodes,
		namespaceClient: namespaces,
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
	extraInfo, err := s.getExtraInfo()
	if err != nil {
		return err
	}
	req := &CheckUpgradeRequest{
		AppVersion: settings.ServerVersion.Get(),
		ExtraInfo:  extraInfo,
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

func (s *versionSyncer) getExtraInfo() (map[string]string, error) {
	nodes, err := s.nodeClient.List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	sysNamespace, err := s.namespaceClient.Get(util.HarvesterSystemNamespaceName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	extraInfo := map[string]string{}
	cpu := resource.NewQuantity(0, resource.BinarySI)
	memory := resource.NewQuantity(0, resource.BinarySI)
	for _, node := range nodes.Items {
		cpu.Add(*node.Status.Capacity.Cpu())
		memory.Add(*node.Status.Capacity.Memory())
	}
	extraInfo[extraInfoCPUCount] = cpu.String()
	extraInfo[extraInfoMemorySize] = formatQuantityToGi(memory)
	extraInfo[extraInfoNodeCount] = strconv.Itoa(len(nodes.Items))
	extraInfo[extraInfoClusterUID] = string(sysNamespace.UID)
	return extraInfo, nil
}

func (s *versionSyncer) syncVersions(resp CheckUpgradeResponse, currentVersion string) error {
	if err := s.cleanupVersions(currentVersion, resp.Versions); err != nil {
		return err
	}

	// iterate over response and identify if a new version needs to be created
	for _, v := range resp.Versions {
		newVersion, err := s.getNewVersion(v)
		if err != nil {
			if strings.Contains(err.Error(), "failed to download version") {
				continue
			}
			return err
		}

		// newVersion CRD is created from the version.yaml created from the release artifact
		// this needs to be changed to use the min version coming from the upgrade responder
		// as this could be dynamically update and will cause cleanup of older versions
		if !canUpgrade(currentVersion, newVersion, v) {
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
func (s *versionSyncer) cleanupVersions(currentVersion string, remoteVersions []Version) error {
	versionList, err := s.versionClient.List(s.namespace, metav1.ListOptions{})
	if err != nil {
		return err
	}

	remainingVersions := versionList.Items

	for _, remote := range remoteVersions {
		versions := remainingVersions
		remainingVersions = []harvesterv1.Version{}

		for _, version := range versions {
			if !canUpgrade(currentVersion, &version, remote) {
				if err := s.versionClient.Delete(version.Namespace, version.Name, &metav1.DeleteOptions{}); err != nil {
					if apierrors.IsNotFound(err) {
						continue // likely object has been removed so we can continue
					}
					return err
				}
			} else {
				remainingVersions = append(remainingVersions, version)
			}
		}
	}

	return nil
}

func (s *versionSyncer) getNewVersion(v Version) (*harvesterv1.Version, error) {
	releaseDownloadURL := settings.ReleaseDownloadURL.Get()
	var archSuffix string
	if runtime.GOARCH == "arm64" {
		archSuffix = "-arm64"
	}
	url := fmt.Sprintf("%s/%s/version%s.yaml", releaseDownloadURL, v.Name, archSuffix)
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

func canUpgrade(currentVersion string, newVersion *harvesterv1.Version, responderVersion Version) bool {
	switch {
	case newVersion.Spec.ISOURL == "" || newVersion.Spec.ISOChecksum == "":
		return false
	case slice.ContainsString(responderVersion.Tags, "dev"):
		return true
	case gversion.Compare(currentVersion, responderVersion.Name, "<") && (responderVersion.MinUpgradableVersion == "" || gversion.Compare(currentVersion, responderVersion.MinUpgradableVersion, ">=")):
		return true
	default:
		return false
	}
}

func formatQuantityToGi(q *resource.Quantity) string {
	// 32920204Ki,
	// q.Value(): 32920204*1024=33710288896
	// math.Pow(1024, 3): 1024*1024*1024=1073741824
	// float64(q.Value())/math.Pow(1024, 3): 33710288896/1073741824=31.3951530456543
	// math.Ceil(31.3951530456543)=32
	giB := 1024 * 1024 * 1024
	return fmt.Sprintf("%dGi", int64(math.Ceil(float64(q.Value())/float64(giB))))
}
