package setting

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rancher/wrangler/v3/pkg/objectset"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	harvsettings "github.com/harvester/harvester/pkg/settings"
)

const defaultResponseMaxSize = 1 * 1024 * 1024 // 1MB

// registerCluster imports Harvester to Rancher by applying manifests from the registration URL.
func (h *Handler) registerCluster(setting *harvesterv1.Setting) error {
	logrus.Info("syncing cluster registration URL setting")
	s := harvsettings.GetClusterRegistrationURLSetting(setting)
	url := s.URL
	if url == "" {
		logrus.Info("resetting cluster registration URL, cleaning up cluster agent if exists")
		return h.cleanupClusterAgent()
	}

	httpClient := h.httpClient
	if !s.InsecureSkipTLSVerify {
		logrus.Info("using system CA certificates to verify the cluster registration URL")
		caCertPool, err := x509.SystemCertPool()
		if err != nil {
			return err
		}
		httpClient = http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				TLSClientConfig: &tls.Config{
					RootCAs: caCertPool,
				},
			},
		}
	}

	resp, err := httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	limitReader := io.LimitReader(resp.Body, defaultResponseMaxSize)
	body, err := io.ReadAll(limitReader)
	if err != nil {
		return err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("failed to get from the registration URL: %s", body)
	}

	var (
		objects        = objectset.NewObjectSet()
		multidocReader = utilyaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(body)))
	)
	for {
		buf, err := multidocReader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		// Skip the YAML doc if it is unkind.
		var typeMeta runtime.TypeMeta
		if err := yaml.Unmarshal(buf, &typeMeta); err != nil || typeMeta.Kind == "" {
			continue
		}

		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{},
		}
		if err := yaml.Unmarshal(buf, &obj.Object); err != nil {
			return err
		}
		objects.Add(obj)
	}
	return h.apply.
		WithDynamicLookup().
		WithSetID("cluster-registration").
		Apply(objects)
}

func (h *Handler) cleanupClusterAgent() error {
	// cleanup rancher related resources
	// ref: https://rancher.com/docs/rancher/v2.6/en/cluster-provisioning/registered-clusters/#registering-a-cluster
	// ref: https://rancher.com/docs/rancher/v2.6/en/cluster-provisioning/rke-clusters/rancher-agents/
	if _, err := h.deploymentCache.Get("cattle-system", "cattle-cluster-agent"); err == nil {
		if err = h.deployments.Delete("cattle-system", "cattle-cluster-agent", nil); err != nil {
			logrus.Errorf("Can't delete cattle-system/cattle-cluster-agent deployment, err: %+v", err)
			return err
		}
	} else if err != nil && !apierrors.IsNotFound(err) {
		logrus.Errorf("Can't get cattle-system/cattle-cluster-agent deployment, err: %+v", err)
		return err
	}
	return nil
}
