package namespace

import (
	"fmt"

	"github.com/rancher/apiserver/pkg/types"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"

	supportBundleUtil "github.com/harvester/harvester/pkg/util/supportbundle"
)

const (
	queryParamLink        = "link"
	linkTypeSupportBundle = "supportbundle"
)

type Store struct {
	types.Store
	nsCache ctlcorev1.NamespaceCache
}

func (s *Store) List(req *types.APIRequest, schema *types.APISchema) (types.APIObjectList, error) {
	linkType := s.getLinkQueryType(req)
	if linkType == "" {
		return s.Store.List(req, schema)
	}

	switch linkType {
	case linkTypeSupportBundle:
		return s.handleSupportBundleRequest()
	default:
		return s.Store.List(req, schema)
	}
}

// getLinkQueryType extracts and validates the link query parameter
func (s *Store) getLinkQueryType(req *types.APIRequest) string {
	queryValues, exists := req.Query[queryParamLink]
	if !exists || len(queryValues) == 0 {
		return ""
	}
	return queryValues[0]
}

// handleSupportBundleRequest handles the supportbundle link type request
func (s *Store) handleSupportBundleRequest() (types.APIObjectList, error) {
	objects := s.getSupportBundleUsedNamespaceObj()
	return types.APIObjectList{Objects: objects, Count: len(objects)}, nil
}

func (s *Store) getSupportBundleUsedNamespaceObj() []types.APIObject {
	defaultNamespaces := supportBundleUtil.DefaultNamespaces()
	apiObjs := make([]types.APIObject, 0, len(defaultNamespaces))

	for _, namespaceName := range defaultNamespaces {
		nsObj, err := s.nsCache.Get(namespaceName)
		if err != nil {
			fmt.Printf("Failed to get namespace %s: %v\n", namespaceName, err)
			continue
		}

		apiObjs = append(apiObjs, types.APIObject{
			Type:   "namespace",
			ID:     namespaceName,
			Object: nsObj,
		})
	}

	return apiObjs
}
