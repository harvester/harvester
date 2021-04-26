package user

import (
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/wrangler/pkg/schemas/validation"

	ctlharvesterv1 "github.com/rancher/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/rancher/harvester/pkg/indexeres"
	pkguser "github.com/rancher/harvester/pkg/user"
)

const (
	FieldPassword = "password"
	FieldUsername = "username"
)

type userStore struct {
	types.Store
	mu        sync.Mutex
	userCache ctlharvesterv1.UserCache
}

func (s *userStore) Create(request *types.APIRequest, schema *types.APISchema, data types.APIObject) (types.APIObject, error) {
	newData := data.Data()
	username := newData.String(FieldUsername)
	if username == "" {
		return types.APIObject{}, apierror.NewAPIError(validation.InvalidBodyContent, "Username %s is empty")
	}

	passwordPlainText := newData.String(FieldPassword)
	password, err := pkguser.HashPasswordString(passwordPlainText)
	if err != nil {
		return types.APIObject{}, apierror.NewAPIError(validation.ServerError, "Failed to encrypt password")
	}

	newData.Set(FieldPassword, password)
	newData.SetNested(generateUserObjectName(username), "metadata", "name")
	data.Object = newData

	created, err := s.create(request, schema, data, username)
	if err != nil {
		return types.APIObject{}, apierror.NewAPIError(validation.ServerError, "Failed to create user, "+err.Error())
	}

	delete(created.Data(), "password")
	return created, nil
}

func (s *userStore) create(request *types.APIRequest, schema *types.APISchema, data types.APIObject, username string) (types.APIObject, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	users, err := s.userCache.GetByIndex(indexeres.UserNameIndex, username)
	if err != nil {
		return types.APIObject{}, err
	}
	if len(users) > 0 {
		return types.APIObject{}, errors.New("username is already in use")
	}

	return s.Store.Create(request, schema, data)
}

func (s *userStore) Update(request *types.APIRequest, schema *types.APISchema, data types.APIObject, id string) (types.APIObject, error) {
	newData := data.Data()
	passwordPlainText := newData.String(FieldPassword)
	password, err := pkguser.HashPasswordString(passwordPlainText)
	if err != nil {
		return types.APIObject{}, apierror.NewAPIError(validation.ServerError, "Failed to encrypt password")
	}
	newData.Set(FieldPassword, password)
	data.Object = newData

	updated, err := s.Store.Update(request, request.Schema, data, id)
	if err != nil {
		return types.APIObject{}, apierror.NewAPIError(validation.ServerError, "Failed to update user, "+err.Error())
	}

	delete(updated.Data(), "password")
	return updated, nil
}

func (s *userStore) Delete(request *types.APIRequest, schema *types.APISchema, id string) (types.APIObject, error) {
	currentUser := request.GetUser()
	if currentUser == id {
		return types.APIObject{}, apierror.NewAPIError(validation.ServerError, "You cannot delete yourself")
	}

	return s.Store.Delete(request, request.Schema, id)
}

func generateUserObjectName(username string) string {
	// Create a hash of the userName to use as the name for the user,
	// this lets k8s tell us if there are duplicate users with the same name
	// thus avoiding a race.
	h := sha256.New()
	_, _ = h.Write([]byte(username))
	sha := base32.StdEncoding.WithPadding(-1).EncodeToString(h.Sum(nil))[:10]
	return fmt.Sprintf("u-" + strings.ToLower(sha))
}
