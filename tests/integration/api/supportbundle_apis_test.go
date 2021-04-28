package api_test

import (
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	. "github.com/harvester/harvester/tests/framework/dsl"
	"github.com/harvester/harvester/tests/framework/fuzz"
	"github.com/harvester/harvester/tests/framework/helper"
)

// A workaround to drop the `description` key when marshalling. The `omitempty`
// tag conflicts with the `+kubebuilder:validation:Required` marker.
type InvalidSpec struct {
	IssueURL    string `json:"issueURL,omitempty"`
	Description string `json:"description,omitempty"`
}

type InvalidSupportBundle struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec InvalidSpec `json:"spec,omitempty"`
}

var _ = Describe("verify supportbundle APIs", func() {

	var sbNamespace string

	BeforeEach(func() {

		sbNamespace = "harvester-system"

	})

	Context("operate via steve API", func() {

		var sbAPI string

		BeforeEach(func() {

			sbAPI = helper.BuildAPIURL("v1", "harvesterhci.io.supportbundles", options.HTTPSListenPort)

		})

		Specify("verify required fields", func() {

			By("creating a supportbundle without description", func() {

				sb := InvalidSupportBundle{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fuzz.String(5),
						Namespace: sbNamespace,
					},
					Spec: InvalidSpec{},
				}

				respCode, respBody, err := helper.PostObject(sbAPI, sb)
				MustRespCodeIs(http.StatusUnprocessableEntity, "post supportbundle", err, respCode, respBody)
			})
		})

		Specify("verify supportbundle ready", func() {

			var (
				sbName = fuzz.String(5)
				sb     = harvesterv1.SupportBundle{
					ObjectMeta: metav1.ObjectMeta{
						Name:      sbName,
						Namespace: sbNamespace,
					},
					Spec: harvesterv1.SupportBundleSpec{
						IssueURL:    "http://to.my/issue",
						Description: "desc",
					},
				}
				sbURL = fmt.Sprintf("%s/%s/%s", sbAPI, sbNamespace, sbName)
				retSb harvesterv1.SupportBundle
			)

			By("creating a supportbundle", func() {
				respCode, respBody, err := helper.PostObject(sbAPI, sb)
				MustRespCodeIs(http.StatusCreated, "post supportbundle", err, respCode, respBody)
			})

			By("waiting for Initialized condition to be true")
			MustFinallyBeTrue(func() bool {
				respCode, respBody, err := helper.GetObject(sbURL, &retSb)
				MustRespCodeIs(http.StatusOK, "get supportbundle", err, respCode, respBody)
				Expect(harvesterv1.SupportBundleInitialized.IsFalse(retSb)).NotTo(BeTrue())
				return harvesterv1.SupportBundleInitialized.IsTrue(retSb)
			}, 2*time.Minute, 3*time.Second)

			By("checking filename and filesize is not empty", func() {
				respCode, respBody, err := helper.GetObject(sbAPI, &retSb)
				MustRespCodeIs(http.StatusOK, "get supportbundle", err, respCode, respBody)
				Expect(retSb.Status.Filename).NotTo(BeEmpty())
				Expect(retSb.Status.Filesize).NotTo(BeZero())
			})

			By("deleting the supportbundle", func() {
				respCode, respBody, err := helper.DeleteObject(sbURL)
				MustRespCodeIn("delete supportbundle", err, respCode, respBody, http.StatusOK, http.StatusNoContent)
			})
		})
	})
})
