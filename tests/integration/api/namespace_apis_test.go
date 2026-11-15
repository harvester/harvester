package api_test

import (
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/config"
	supportBundleUtil "github.com/harvester/harvester/pkg/util/supportbundle"
	"github.com/harvester/harvester/tests/framework/fuzz"
	"github.com/harvester/harvester/tests/framework/helper"
)

const namespaceUpdatedLabel = "harvester.test.io/updated"

// The namespace store of the API server wraps the default store, so create, update and delete
// have to work the same with and without the SQL cache.
var _ = Describe("verify namespace APIs", ginkgo.Ordered, func() {

	var (
		scaled    *config.Scaled
		nsAPI     string
		nsURL     string
		namespace string
	)

	namespaceIDs := func() []string {
		code, resp, body := fetchList(nsAPI, "", nil)
		Expect(code).To(Equal(http.StatusOK), body)
		return resp.ids()
	}

	ginkgo.BeforeAll(func() {
		scaled = harvester.Scaled()
		nsAPI = helper.BuildAPIURL("v1", "namespaces", options.HTTPSListenPort)
		namespace = "api-ns-" + fuzz.String(5)
		nsURL = nsAPI + "/" + namespace
	})

	ginkgo.AfterAll(func() {
		err := scaled.CoreFactory.Core().V1().Namespace().Delete(namespace, &metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			GinkgoT().Logf("failed to delete test namespace %s, %v", namespace, err)
		}
	})

	Specify("create a namespace", func() {
		body := map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name":   namespace,
				"labels": testResourceLabels,
			},
		}
		respCode, respBody, err := helper.PostObject(nsAPI, body)
		MustRespCodeIs(http.StatusCreated, "create namespace", err, respCode, respBody)

		ns, err := scaled.CoreFactory.Core().V1().Namespace().Get(namespace, metav1.GetOptions{})
		MustNotError(err)
		Expect(ns.Labels).To(gomega.HaveKeyWithValue("harvester.test.io", "harvester-test"))
	})

	Specify("the namespace is found by id and in the list", func() {
		MustFinallyBeTrue(func() bool {
			var object struct {
				ID string `json:"id"`
			}
			respCode, _, err := helper.GetObject(nsURL, &object)
			return err == nil && respCode == http.StatusOK && object.ID == namespace
		})
		MustFinallyBeTrue(func() bool {
			for _, id := range namespaceIDs() {
				if id == namespace {
					return true
				}
			}
			return false
		})
	})

	Specify("an unknown namespace is not found", func() {
		respCode, _, err := helper.GetObject(nsURL+"-unknown", nil)
		MustNotError(err)
		Expect(respCode).To(Equal(http.StatusNotFound))
	})

	Specify("update the labels of the namespace", func() {
		MustFinallyBeTrue(func() bool {
			ns, err := scaled.CoreFactory.Core().V1().Namespace().Get(namespace, metav1.GetOptions{})
			MustNotError(err)
			ns = ns.DeepCopy()
			ns.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Namespace"}
			if ns.Labels == nil {
				ns.Labels = map[string]string{}
			}
			ns.Labels[namespaceUpdatedLabel] = "true"
			respCode, respBody, err := helper.PutObject(nsURL, ns)
			MustNotError(err)
			// 409 may occur when a controller changes the namespace at the same time
			Expect(respCode).To(BeElementOf([]int{http.StatusOK, http.StatusConflict}), string(respBody))
			return respCode == http.StatusOK
		}, 10*time.Second, 2*time.Second)

		ns, err := scaled.CoreFactory.Core().V1().Namespace().Get(namespace, metav1.GetOptions{})
		MustNotError(err)
		Expect(ns.Labels).To(gomega.HaveKeyWithValue(namespaceUpdatedLabel, "true"))

		MustFinallyBeTrue(func() bool {
			var object struct {
				Metadata struct {
					Labels map[string]string `json:"labels"`
				} `json:"metadata"`
			}
			respCode, _, err := helper.GetObject(nsURL, &object)
			return err == nil && respCode == http.StatusOK && object.Metadata.Labels[namespaceUpdatedLabel] == "true"
		})
	})

	Specify("delete the namespace", func() {
		respCode, respBody, err := helper.DeleteObject(nsURL)
		MustRespCodeIs(http.StatusOK, "delete namespace", err, respCode, respBody)

		MustFinallyBeTrue(func() bool {
			_, err := scaled.CoreFactory.Core().V1().Namespace().Get(namespace, metav1.GetOptions{})
			return errors.IsNotFound(err)
		}, 120*time.Second, 2*time.Second)
		MustFinallyBeTrue(func() bool {
			for _, id := range namespaceIDs() {
				if id == namespace {
					return false
				}
			}
			return true
		}, 30*time.Second, 2*time.Second)
	})

	Specify("the list with link=supportbundle only has the default namespaces that exist", func() {
		var expected []string
		for _, name := range supportBundleUtil.DefaultNamespaces() {
			if _, err := scaled.CoreFactory.Core().V1().Namespace().Get(name, metav1.GetOptions{}); err == nil {
				expected = append(expected, name)
			}
		}
		sort.Strings(expected)
		// the test setup creates harvester-system and longhorn-system
		Expect(expected).NotTo(BeEmpty())

		code, resp, body := fetchList(nsAPI, "", url.Values{"link": {"supportbundle"}})
		Expect(code).To(Equal(http.StatusOK), body)
		got := resp.ids()
		sort.Strings(got)
		Expect(got).To(Equal(expected))
		Expect(resp.Count).To(Equal(len(expected)))
	})
})
