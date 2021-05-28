package user

import (
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/indexeres"
	werror "github.com/harvester/harvester/pkg/webhook/error"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func NewValidator(users ctlharvesterv1.UserCache) types.Validator {
	users.AddIndexer(indexeres.UserNameIndex, indexeres.IndexUserByUsername)
	return &userValidator{users: users}
}

type userValidator struct {
	types.DefaultValidator

	users ctlharvesterv1.UserCache
}

func (v *userValidator) Resource() types.Resource {
	return newResource([]admissionregv1.OperationType{
		admissionregv1.Create,
		admissionregv1.Update,
		admissionregv1.Delete,
	})
}

func (v *userValidator) Create(request *types.Request, newObj runtime.Object) error {
	user := newObj.(*v1beta1.User)
	return v.createUpdateUser(user, true)
}

func (v *userValidator) Update(request *types.Request, oldObj runtime.Object, newObj runtime.Object) error {
	user := newObj.(*v1beta1.User)
	return v.createUpdateUser(user, false)
}

func (v *userValidator) Delete(request *types.Request, oldObj runtime.Object) error {
	user := oldObj.(*v1beta1.User)
	if user.Name == request.AdmissionRequest.UserInfo.Username {
		return werror.NewInvalidError("can't delete self", "metadata.name")
	}
	return nil
}

func (v *userValidator) createUpdateUser(user *v1beta1.User, create bool) error {
	if user.Username == "" {
		return werror.NewInvalidError("username is required", "username")
	}

	if user.Password == "" {
		return werror.NewInvalidError("password is required", "password")
	}

	if create {
		users, err := v.users.GetByIndex(indexeres.UserNameIndex, user.Username)
		if err != nil {
			return err
		}
		if len(users) > 0 {
			return werror.NewConflict("username is already in use")
		}
	}

	return nil
}
