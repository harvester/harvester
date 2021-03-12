package userpreferences

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
	"github.com/rancher/apiserver/pkg/store/empty"
	"github.com/rancher/apiserver/pkg/types"
	"k8s.io/apiserver/pkg/endpoints/request"
)

var (
	rancherSchema = "management.cattle.io.preference"
)

type localStore struct {
	empty.Store
}

func confDir() string {
	return filepath.Join(xdg.ConfigHome, "steve")
}

func confFile() string {
	return filepath.Join(confDir(), "prefs.json")
}

func set(data map[string]interface{}) error {
	if err := os.MkdirAll(confDir(), 0700); err != nil {
		return err
	}
	bytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(confFile(), bytes, 0600)
}

func get() (map[string]string, error) {
	data := UserPreference{}
	f, err := os.Open(confFile())
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	} else if err != nil {
		return nil, err
	}
	defer f.Close()

	if err := json.NewDecoder(f).Decode(&data); err != nil {
		return nil, err
	}
	return data.Data, nil
}

func getUserName(apiOp *types.APIRequest) string {
	user, ok := request.UserFrom(apiOp.Context())
	if !ok {
		return "local"
	}
	return user.GetName()
}

func (l *localStore) ByID(apiOp *types.APIRequest, schema *types.APISchema, id string) (types.APIObject, error) {
	data, err := get()
	if err != nil {
		return types.APIObject{}, err
	}

	return types.APIObject{
		Type: "userpreference",
		ID:   getUserName(apiOp),
		Object: UserPreference{
			Data: data,
		},
	}, nil
}

func (l *localStore) List(apiOp *types.APIRequest, schema *types.APISchema) (types.APIObjectList, error) {
	obj, err := l.ByID(apiOp, schema, "")
	if err != nil {
		return types.APIObjectList{}, err
	}
	return types.APIObjectList{
		Objects: []types.APIObject{
			obj,
		},
	}, nil
}

func (l *localStore) Update(apiOp *types.APIRequest, schema *types.APISchema, data types.APIObject, id string) (types.APIObject, error) {
	err := set(data.Data())
	if err != nil {
		return types.APIObject{}, err
	}
	return l.ByID(apiOp, schema, "")
}

func (l *localStore) Delete(apiOp *types.APIRequest, schema *types.APISchema, id string) (types.APIObject, error) {
	return l.Update(apiOp, schema, types.APIObject{
		Object: map[string]interface{}{},
	}, "")
}
