package keypair

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/wrangler/v3/pkg/schemas/validation"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	harvesterServer "github.com/harvester/harvester/pkg/server/http"
	"github.com/harvester/harvester/pkg/util"
)

func Formatter(_ *types.APIRequest, resource *types.RawResource) {
	resource.Actions = nil
	delete(resource.Links, "update")
}

func CollectionFormatter(request *types.APIRequest, collection *types.GenericCollection) {
	collection.AddAction(request, "keygen")
}

type KeyGenActionHandler struct {
	KeyPairs     ctlharvesterv1.KeyPairClient
	KeyPairCache ctlharvesterv1.KeyPairCache
}

func (h KeyGenActionHandler) Do(ctx *harvesterServer.Ctx) (harvesterServer.ResponseBody, error) {
	req, rw := ctx.Req(), ctx.RespWriter()

	input := &harvesterv1.KeyGenInput{}
	if err := json.NewDecoder(req.Body).Decode(input); err != nil {
		return nil, apierror.NewAPIError(validation.InvalidBodyContent, fmt.Sprintf("Failed to parse body: %v", err))
	}
	if input.Name == "" || input.Namespace == "" {
		return nil, apierror.NewAPIError(validation.InvalidBodyContent, "both name and namespace is required")
	}
	rsaKey, err := util.GeneratePrivateKey(2048)
	if err != nil {
		return nil, err
	}
	privateKey := util.EncodePrivateKeyToPEM(rsaKey)
	publicKey, err := util.GeneratePublicKey(&rsaKey.PublicKey)
	if err != nil {
		return nil, err
	}

	keyPair := &harvesterv1.KeyPair{
		ObjectMeta: v1.ObjectMeta{
			Name:      input.Name,
			Namespace: input.Namespace,
		},
		Spec: harvesterv1.KeyPairSpec{
			PublicKey: string(publicKey),
		},
	}

	if _, err = h.KeyPairs.Create(keyPair); err != nil {
		return nil, err
	}

	rw.Header().Set("Content-Disposition", "attachment; filename="+input.Name+".pem")
	rw.Header().Set("Content-Type", "application/octet-stream")
	rw.Header().Set("Content-Length", strconv.Itoa(len(privateKey)))
	_, err = rw.Write(privateKey)
	return nil, err
}
