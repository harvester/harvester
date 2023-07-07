package server

import (
	"net/http"
	"reflect"

	"github.com/rancher/wrangler/pkg/webhook"
	"github.com/sirupsen/logrus"

	"github.com/harvester/harvester/pkg/webhook/clients"
	"github.com/harvester/harvester/pkg/webhook/config"
	"github.com/harvester/harvester/pkg/webhook/resources/pod"
	"github.com/harvester/harvester/pkg/webhook/resources/templateversion"
	"github.com/harvester/harvester/pkg/webhook/resources/virtualmachine"
	"github.com/harvester/harvester/pkg/webhook/resources/virtualmachineimage"
	"github.com/harvester/harvester/pkg/webhook/types"
)

func Mutation(clients *clients.Clients, options *config.Options) (http.Handler, []types.Resource, error) {
	resources := []types.Resource{}
	settingCache := clients.HarvesterFactory.Harvesterhci().V1beta1().Setting().Cache()
	storageClassCache := clients.StorageFactory.Storage().V1().StorageClass().Cache()
	nadCache := clients.CNIFactory.K8s().V1().NetworkAttachmentDefinition().Cache()
	mutators := []types.Mutator{
		pod.NewMutator(settingCache),
		templateversion.NewMutator(),
		virtualmachine.NewMutator(settingCache, nadCache),
		virtualmachineimage.NewMutator(storageClassCache),
	}

	router := webhook.NewRouter()
	for _, m := range mutators {
		addHandler(router, types.AdmissionTypeMutation, m, options)
		resources = append(resources, m.Resource())
	}

	return router, resources, nil
}

func addHandler(router *webhook.Router, admissionType string, admitter types.Admitter, options *config.Options) {
	rsc := admitter.Resource()
	kind := reflect.Indirect(reflect.ValueOf(rsc.ObjectType)).Type().Name()
	router.Kind(kind).Group(rsc.APIGroup).Type(rsc.ObjectType).Handle(types.NewAdmissionHandler(admitter, admissionType, options))
	logrus.Infof("add %s handler for %+v.%s (%s)", admissionType, rsc.Names, rsc.APIGroup, kind)
}
