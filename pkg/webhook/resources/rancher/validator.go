package rancher

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/harvester/pkg/webhook/types"
)

const (
	podNameKey = "authentication.kubernetes.io/pod-name"
)

var (
	whitelistedPodPrefixes = []string{"rancher", "fleet", "harvester", "longhorn", "pcidevices", "node-manager"}
)

type rancherValidator struct {
	types.DefaultValidator
}

func (r *rancherValidator) Create(request *types.Request, newObj runtime.Object) error {
	if !requestIsNotFromInfra(request) {
		return fmt.Errorf("user %s cannot perform action", request.Request.UserInfo.Username)
	}
	return nil
}

func (r *rancherValidator) Update(request *types.Request, oldObj, newObj runtime.Object) error {
	if !requestIsNotFromInfra(request) {
		return fmt.Errorf("user %s cannot perform action", request.Request.UserInfo.Username)
	}
	return nil
}

func (r *rancherValidator) Delete(request *types.Request, oldObj runtime.Object) error {
	if !requestIsNotFromInfra(request) {
		return fmt.Errorf("user %s cannot perform action", request.Request.UserInfo.Username)
	}
	return nil
}

func (r *rancherValidator) Connect(request *types.Request, oldObj runtime.Object) error {
	if !requestIsNotFromInfra(request) {
		return fmt.Errorf("user %s cannot perform action", request.Request.UserInfo.Username)
	}
	return nil
}

// query extra info, for requests coming from pods this is set.
func requestIsNotFromInfra(request *types.Request) bool {
	logrus.Info(request.UserInfo)
	if request.Request.UserInfo.Extra == nil {
		return false
	}

	if requestFromUI(request) {
		return true
	}

	return requestFromControllers(request)
}

// request from UI will have extra info for principalid or username populated
func requestFromUI(request *types.Request) bool {
	_, princpalidok := request.Request.UserInfo.Extra["principalid"]

	_, usernameok := request.Request.UserInfo.Extra["username"]

	return princpalidok || usernameok
}

func requestFromControllers(request *types.Request) bool {
	if val, ok := request.Request.UserInfo.Extra[podNameKey]; ok {
		for _, v := range whitelistedPodPrefixes {
			if strings.Contains(val[0], v) {
				return true
			}
		}
	}
	return false
}
