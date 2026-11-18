package api_test

import (
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester/pkg/config"
	"github.com/harvester/harvester/tests/framework/env"
	"github.com/harvester/harvester/tests/framework/fuzz"
	"github.com/harvester/harvester/tests/framework/helper"
)

const lookupGroupLabel = "harvester.test.io/group"

// These are the queries the UI uses to look up objects: a filter on a label and a filter with
// "in". They are part of the filter language of the SQL cache, the previous backend does not
// know them.
var _ = Describe("verify lookup queries", ginkgo.Ordered, func() {

	var (
		scaled     *config.Scaled
		configMaps string
		namespaces string
		testNS     string
	)

	// configMapNames returns the sorted names of the config maps of the test namespace that match the query.
	configMapNames := func(filter string) []string {
		code, resp, body := fetchList(configMaps, testNS, url.Values{"filter": {filter}, "sort": {"metadata.name"}})
		Expect(code).To(Equal(200), body)
		var names []string
		for _, id := range resp.ids() {
			if name, found := strings.CutPrefix(id, testNS+"/"); found {
				names = append(names, name)
			}
		}
		return names
	}

	ginkgo.BeforeAll(func() {
		if !options.SQLCache {
			ginkgo.Skip("needs the SQL cache (default, HARVESTER_SQL_CACHE=true)")
		}
		scaled = harvester.Scaled()
		configMaps = helper.BuildAPIURL("v1", "configmaps", options.HTTPSListenPort)
		namespaces = helper.BuildAPIURL("v1", "namespaces", options.HTTPSListenPort)

		testNS = "lookup-" + fuzz.String(5)
		_, err := scaled.CoreFactory.Core().V1().Namespace().Create(&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: testNS, Labels: testResourceLabels},
		})
		MustNotError(err)

		for name, group := range map[string]string{"cm-a1": "a", "cm-a2": "a", "cm-b1": "b", "cm-none": ""} {
			labels := map[string]string{}
			if group != "" {
				labels[lookupGroupLabel] = group
			}
			_, err := scaled.CoreFactory.Core().V1().ConfigMap().Create(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNS, Labels: labels},
			})
			MustNotError(err)
		}

		MustFinallyBeTrue(func() bool {
			code, resp, _ := fetchList(configMaps, testNS, nil)
			if code != 200 {
				return false
			}
			ids := resp.ids()
			for _, name := range []string{"cm-a1", "cm-a2", "cm-b1", "cm-none"} {
				if !slices.Contains(ids, testNS+"/"+name) {
					return false
				}
			}
			return true
		}, 30*time.Second, 2*time.Second)
	})

	ginkgo.AfterAll(func() {
		if scaled == nil || env.IsKeepingTestingResource() {
			return
		}
		err := scaled.CoreFactory.Core().V1().Namespace().Delete(testNS, &metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			GinkgoT().Logf("failed to delete test namespace %s, %v", testNS, err)
		}
	})

	Specify("filter by the value of a label", func() {
		Expect(configMapNames("metadata.labels[" + lookupGroupLabel + "]=a")).To(Equal([]string{"cm-a1", "cm-a2"}))
		Expect(configMapNames("metadata.labels[" + lookupGroupLabel + "]=b")).To(Equal([]string{"cm-b1"}))
	})

	Specify("filter by a label with in", func() {
		Expect(configMapNames("metadata.labels[" + lookupGroupLabel + "] in (a,b)")).
			To(Equal([]string{"cm-a1", "cm-a2", "cm-b1"}))
	})

	Specify("filter by the name with in", func() {
		Expect(configMapNames("metadata.name in (cm-a1,cm-none)")).To(Equal([]string{"cm-a1", "cm-none"}))
	})

	Specify("filter a cluster scoped resource by the name with in", func() {
		// the test setup creates both namespaces
		code, resp, body := fetchList(namespaces, "", url.Values{
			"filter": {"metadata.name in (harvester-system,longhorn-system)"},
			"sort":   {"metadata.name"},
		})
		Expect(code).To(Equal(200), body)
		Expect(resp.ids()).To(Equal([]string{"harvester-system", "longhorn-system"}))
	})
})
