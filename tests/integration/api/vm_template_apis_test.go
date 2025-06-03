package api_test

import (
	"fmt"
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/tests/framework/fuzz"
	"github.com/harvester/harvester/tests/framework/helper"
)

const (
	defaultVMTemplates        = 4
	defaultVMTemplateVersions = 4
)

var _ = Describe("verify vm template APIs", func() {

	var (
		scaled            *config.Scaled
		templates         ctlharvesterv1.VirtualMachineTemplateClient
		templateVersions  ctlharvesterv1.VirtualMachineTemplateVersionClient
		templateNamespace string
	)

	BeforeEach(func() {
		scaled = harvester.Scaled()
		templates = scaled.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineTemplate()
		templateVersions = scaled.HarvesterFactory.Harvesterhci().V1beta1().VirtualMachineTemplateVersion()
		templateNamespace = options.Namespace
	})

	Cleanup(func() {
		templateList, err := templates.List(templateNamespace, metav1.ListOptions{
			LabelSelector: labels.FormatLabels(testResourceLabels)})
		if err != nil {
			GinkgoT().Logf("failed to list tested vm templates, %v", err)
			return
		}
		for _, item := range templateList.Items {
			if err = templates.Delete(item.Namespace, item.Name, &metav1.DeleteOptions{}); err != nil {
				GinkgoT().Logf("failed to delete tested template %s/%s, %v", item.Namespace, item.Name, err)
			}
		}

		templateVersionList, err := templateVersions.List(templateNamespace, metav1.ListOptions{
			LabelSelector: labels.FormatLabels(testResourceLabels)})
		if err != nil {
			GinkgoT().Logf("failed to list tested vm templates, %v", err)
			return
		}
		for _, item := range templateVersionList.Items {
			if err = templateVersions.Delete(item.Namespace, item.Name, &metav1.DeleteOptions{}); err != nil {
				GinkgoT().Logf("failed to delete tested template version %s/%s, %v", item.Namespace, item.Name, err)
			}
		}
	})

	Context("operate via steve API", func() {
		var (
			template = harvesterv1.VirtualMachineTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vm-template-0",
					Namespace: "harvester-system",
					Labels:    testResourceLabels,
				},
				Spec: harvesterv1.VirtualMachineTemplateSpec{
					Description: "testing vm template",
				},
			}
			templateVersion = harvesterv1.VirtualMachineTemplateVersion{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fuzz.String(5),
					Namespace: "harvester-system",
					Labels:    testResourceLabels,
				},
				Spec: harvesterv1.VirtualMachineTemplateVersionSpec{},
			}
			templateAPI, templateVersionAPI string
		)

		BeforeEach(func() {
			var port = options.HTTPSListenPort
			templateAPI = helper.BuildAPIURL("v1", "harvesterhci.io.virtualmachinetemplates", port)
			templateVersionAPI = helper.BuildAPIURL("v1", "harvesterhci.io.virtualmachinetemplateversions", port)

		})

		Specify("verify default vm templates", func() {

			By("list default templates", func() {

				templates, respCode, respBody, err := helper.GetCollection(templateAPI)
				MustRespCodeIs(http.StatusOK, "get templates", err, respCode, respBody)
				MustEqual(len(templates.Data), defaultVMTemplates)

			})

			By("list default template versions", func() {

				templates, respCode, respBody, err := helper.GetCollection(templateVersionAPI)
				MustRespCodeIs(http.StatusOK, "get template versions", err, respCode, respBody)
				MustEqual(len(templates.Data), defaultVMTemplateVersions)

			})

		})

		Specify("verify the vm template and template versions", func() {

			var (
				templateID         = fmt.Sprintf("%s/%s", templateNamespace, template.Name)
				defaultVersionID   = fmt.Sprintf("%s/%s", templateNamespace, templateVersion.Name)
				templateURL        = helper.BuildResourceURL(templateAPI, templateNamespace, template.Name)
				templateVersionURL = helper.BuildResourceURL(templateVersionAPI, templateNamespace, templateVersion.Name)
			)

			By("create a vm template", func() {

				respCode, respBody, err := helper.PostObjectByYAML(templateAPI, template)
				MustRespCodeIs(http.StatusCreated, "create template", err, respCode, respBody)

			})

			By("create a vm template version without templateID", func() {

				respCode, respBody, err := helper.PostObjectByYAML(templateVersionAPI, templateVersion)
				MustRespCodeIs(http.StatusUnprocessableEntity, "create template version", err, respCode, respBody)

			})

			By("create a vm template version", func() {

				templateVersion.Spec.TemplateID = templateID
				vm, err := NewDefaultTestVMBuilder(testResourceLabels).VM()
				MustNotError(err)
				templateVersion.Spec.VM = harvesterv1.VirtualMachineSourceSpec{
					ObjectMeta: vm.ObjectMeta,
					Spec:       vm.Spec,
				}

				respCode, respBody, err := helper.PostObjectByYAML(templateVersionAPI, templateVersion)
				MustRespCodeIs(http.StatusCreated, "create template version", err, respCode, respBody)

			})

			By("then validate vm template version", func() {
				MustFinallyBeTrue(func() bool {
					respCode, respBody, err := helper.GetObject(templateURL, &template)
					MustRespCodeIs(http.StatusOK, "get vm template", err, respCode, respBody)
					if template.Spec.DefaultVersionID == defaultVersionID &&
						template.Status.DefaultVersion == 1 && template.Status.LatestVersion == 1 {
						return true
					}
					return false
				}, 3)
			})

			By("can't delete a default template version", func() {

				respCode, respBody, err := helper.DeleteObject(templateVersionURL)
				MustRespCodeIs(http.StatusBadRequest, "delete template version", err, respCode, respBody)

			})

			By("delete a template", func() {

				respCode, respBody, err := helper.DeleteObject(templateURL)
				MustRespCodeIn("delete template", err, respCode, respBody, http.StatusOK, http.StatusNoContent)

			})

			By("then validate total vm templates", func() {

				MustFinallyBeTrue(func() bool {
					templates, respCode, respBody, err := helper.GetCollection(templateAPI)
					MustRespCodeIs(http.StatusOK, "get template", err, respCode, respBody)
					return len(templates.Data) == defaultVMTemplates
				}, 3)

			})

			By("then validate total vm template versions", func() {

				MustFinallyBeTrue(func() bool {
					templates, respCode, respBody, err := helper.GetCollection(templateVersionAPI)
					MustRespCodeIs(http.StatusOK, "get template versions", err, respCode, respBody)
					return len(templates.Data) == defaultVMTemplateVersions
				}, 3)

			})

		})

	})

})
