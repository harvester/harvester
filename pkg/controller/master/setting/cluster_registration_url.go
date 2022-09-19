package setting

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/rancher/wrangler/pkg/objectset"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

// registerCluster imports Harvester to Rancher by applying manifests from the registration URL.
func (h *Handler) registerCluster(setting *harvesterv1.Setting) error {
	url := setting.Value
	if url == "" {
		return h.cleanupClusterAgent()
	}
	resp, err := h.httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
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
	var err error
	if _, err = h.serviceCache.Get("cattle-system", "cattle-cluster-agent"); err == nil {
		if err = h.services.Delete("cattle-system", "cattle-cluster-agent", nil); err != nil {
			logrus.Errorf("Can't delete cattle-system/cattle-cluster-agent service, err: %+v", err)
			return err
		}
	} else if err != nil && !apierrors.IsNotFound(err) {
		logrus.Errorf("Can't get cattle-system/cattle-cluster-agent service, err: %+v", err)
		return err
	}

	if _, err = h.deploymentCache.Get("cattle-system", "cattle-cluster-agent"); err == nil {
		if err = h.deployments.Delete("cattle-system", "cattle-cluster-agent", nil); err != nil {
			logrus.Errorf("Can't delete cattle-system/cattle-cluster-agent deployment, err: %+v", err)
			return err
		}
	} else if err != nil && !apierrors.IsNotFound(err) {
		logrus.Errorf("Can't get cattle-system/cattle-cluster-agent deployment, err: %+v", err)
		return err
	}

	if _, err = h.clusterRoleBindingCache.Get("proxy-role-binding-kubernetes-master"); err == nil {
		if err = h.clusterRoleBindings.Delete("proxy-role-binding-kubernetes-master", nil); err != nil {
			logrus.Errorf("Can't delete proxy-role-binding-kubernetes-master ClusterRoleBinding, err: %+v", err)
			return err
		}
	} else if err != nil && !apierrors.IsNotFound(err) {
		logrus.Errorf("Can't get proxy-role-binding-kubernetes-master ClusterRoleBinding, err: %+v", err)
		return err
	}

	if _, err = h.clusterRoleBindingCache.Get("cattle-admin-binding"); err == nil {
		if err = h.clusterRoleBindings.Delete("cattle-admin-binding", nil); err != nil {
			logrus.Errorf("Can't delete cattle-admin-binding ClusterRoleBinding, err: %+v", err)
			return err
		}
	} else if err != nil && !apierrors.IsNotFound(err) {
		logrus.Errorf("Can't get cattle-admin-binding ClusterRoleBinding, err: %+v", err)
		return err
	}

	if _, err = h.clusterRoleCache.Get("proxy-clusterrole-kubeapiserver"); err == nil {
		if err = h.clusterRoles.Delete("proxy-clusterrole-kubeapiserver", nil); err != nil {
			logrus.Errorf("Can't delete proxy-clusterrole-kubeapiserver ClusterRole, err: %+v", err)
			return err
		}
	} else if err != nil && !apierrors.IsNotFound(err) {
		logrus.Errorf("Can't get proxy-clusterrole-kubeapiserver ClusterRole, err: %+v", err)
		return err
	}

	if _, err = h.clusterRoleCache.Get("cattle-admin"); err == nil {
		if err = h.clusterRoles.Delete("cattle-admin", nil); err != nil {
			logrus.Errorf("Can't delete cattle-admin ClusterRole, err: %+v", err)
			return err
		}
	} else if err != nil && !apierrors.IsNotFound(err) {
		logrus.Errorf("Can't get cattle-admin ClusterRole, err: %+v", err)
		return err
	}

	if _, err = h.serviceAccountCache.Get("cattle-system", "cattle"); err == nil {
		if err = h.serviceAccounts.Delete("cattle-system", "cattle", nil); err != nil {
			logrus.Errorf("Can't delete cattle-admin/cattle ServiceAccount, err: %+v", err)
			return err
		}
	} else if err != nil && !apierrors.IsNotFound(err) {
		logrus.Errorf("Can't get cattle-system/cattle ServiceAccount, err: %+v", err)
		return err
	}

	return nil
}
