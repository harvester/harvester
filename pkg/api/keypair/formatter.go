package keypair

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/steve/pkg/resources/common"
	"github.com/rancher/wrangler/pkg/schemas/validation"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha12 "github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/config"
	"github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
)

func Formatter(request *types.APIRequest, resource *types.RawResource) {
	common.Formatter(request, resource)
	resource.Actions = nil
	delete(resource.Links, "update")
}

func CollectionFormatter(request *types.APIRequest, collection *types.GenericCollection) {
	collection.AddAction(request, "keygen")
}

type KeyGenActionHandler struct {
	KeyPairs     v1alpha1.KeyPairClient
	KeyPairCache v1alpha1.KeyPairCache
}

func (h KeyGenActionHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if err := h.do(rw, req); err != nil {
		status := http.StatusInternalServerError
		if e, ok := err.(*apierror.APIError); ok {
			status = e.Code.Status
		}
		rw.WriteHeader(status)
		_, _ = rw.Write([]byte(err.Error()))
		return
	}
	rw.WriteHeader(http.StatusOK)
}

func (h KeyGenActionHandler) do(rw http.ResponseWriter, req *http.Request) error {
	input := &v1alpha12.KeyGenInput{}
	if err := json.NewDecoder(req.Body).Decode(input); err != nil {
		return apierror.NewAPIError(validation.InvalidBodyContent, fmt.Sprintf("Failed to parse body: %v", err))
	}
	if input.Name == "" {
		return apierror.NewAPIError(validation.InvalidBodyContent, "name is required")
	}
	rsaKey, err := generatePrivateKey(2048)
	if err != nil {
		return err
	}
	privateKey := encodePrivateKeyToPEM(rsaKey)
	publicKey, err := generatePublicKey(&rsaKey.PublicKey)
	if err != nil {
		return err
	}

	keyPair := &v1alpha12.KeyPair{
		ObjectMeta: v1.ObjectMeta{
			Name:      input.Name,
			Namespace: config.Namespace,
		},
		Spec: v1alpha12.KeyPairSpec{
			PublicKey: string(publicKey),
		},
	}

	if _, err = h.KeyPairs.Create(keyPair); err != nil {
		return err
	}

	rw.Header().Set("Content-Disposition", "attachment; filename="+input.Name+".pem")
	rw.Header().Set("Content-Type", "application/octet-stream")
	rw.Header().Set("Content-Length", strconv.Itoa(len(privateKey)))
	_, err = rw.Write(privateKey)
	return err
}
