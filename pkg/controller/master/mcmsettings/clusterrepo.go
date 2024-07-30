package mcmsettings

import (
	"reflect"

	catalogv1 "github.com/rancher/rancher/pkg/apis/catalog.cattle.io/v1"
	"github.com/rancher/wrangler/v3/pkg/slice"
)

const (
	HideClusterRepoKey   = "clusterrepo.cattle.io/hidden"
	HideClusterRepoValue = "true"
)

var (
	// clusterReposToPatch contains the name of clusterrepo objects which need to be patched with the hide annotation
	// values are in the key format
	clusterReposToPatch = []string{"harvester-charts", "rancher-charts", "rancher-rke2-charts", "rancher-stable"}
)

func (h *mcmSettingsHandler) patchClusterRepos(key string, repo *catalogv1.ClusterRepo) (*catalogv1.ClusterRepo, error) {
	if repo == nil || repo.DeletionTimestamp != nil {
		return repo, nil
	}

	if slice.ContainsString(clusterReposToPatch, key) {
		repoCopy := repo.DeepCopy()
		if repoCopy.Annotations == nil {
			repoCopy.Annotations = make(map[string]string)
		}
		repoCopy.Annotations[HideClusterRepoKey] = HideClusterRepoValue
		if !reflect.DeepEqual(repoCopy, repo) {
			return h.clusterRepoClient.Update(repoCopy)
		}
	}
	return repo, nil
}
