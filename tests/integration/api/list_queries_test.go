package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/builder"
	"github.com/harvester/harvester/pkg/config"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/tests/framework/env"
	"github.com/harvester/harvester/tests/framework/fuzz"
	"github.com/harvester/harvester/tests/framework/helper"
)

const (
	displayNameAnnotation = "harvesterhci.io/vmDisplayName"
	displayNameField      = "metadata.annotations[" + displayNameAnnotation + "]"
	statusStopped         = "Stopped"
)

type listResponse struct {
	Count int `json:"count"`
	Data  []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// The expectations are computed from the Kubernetes API (the oracle), not from a previous result of
// the API server. This way the same specs run against both backends of the list API, the one
// without and the one with the SQL cache (HARVESTER_SQL_CACHE=true), and every difference between
// the backends has to be written down in a spec of its own.
var _ = Describe("verify list queries of vm and namespace APIs", ginkgo.Ordered, func() {

	var (
		scaled       *config.Scaled
		vmController ctlkubevirtv1.VirtualMachineController
		vmsAPI       string
		nsAPI        string
		testNS       string
		// the test VMs, sorted by name
		vmNames []string
		// display name of every VM, "Web <ns> NN" for odd numbers and "DB <ns> NN" for even numbers
		displayNames map[string]string
	)

	// ids returns the ids ("namespace/name") of a list request, and the HTTP status code.
	list := func(api, path string, query url.Values) (int, listResponse, string) {
		target := api
		if path != "" {
			target += "/" + path
		}
		if len(query) > 0 {
			target += "?" + query.Encode()
		}
		code, body, err := helper.GetResponse(target)
		MustNotError(err)
		var resp listResponse
		if code == http.StatusOK {
			MustNotError(json.Unmarshal(body, &resp))
		}
		return code, resp, string(body)
	}

	// names returns the names of the test VMs of a list request. Other objects are ignored.
	names := func(query url.Values, path string) []string {
		code, resp, body := list(vmsAPI, path, query)
		Expect(code).To(Equal(http.StatusOK), body)
		var result []string
		for _, item := range resp.Data {
			if name, found := strings.CutPrefix(item.ID, testNS+"/"); found {
				result = append(result, name)
			}
		}
		return result
	}

	q := func(kv ...string) url.Values {
		values := url.Values{}
		for i := 0; i < len(kv); i += 2 {
			values.Add(kv[i], kv[i+1])
		}
		return values
	}

	reversed := func(in []string) []string {
		out := slices.Clone(in)
		slices.Reverse(out)
		return out
	}

	byDisplayName := func(descending bool) []string {
		out := slices.Clone(vmNames)
		slices.SortFunc(out, func(a, b string) int {
			if descending {
				a, b = b, a
			}
			return strings.Compare(displayNames[a], displayNames[b])
		})
		return out
	}

	ginkgo.BeforeAll(func() {
		scaled = harvester.Scaled()
		vmController = scaled.VirtFactory.Kubevirt().V1().VirtualMachine()
		vmsAPI = helper.BuildAPIURL("v1", "kubevirt.io.virtualmachines", options.HTTPSListenPort)
		nsAPI = helper.BuildAPIURL("v1", "namespaces", options.HTTPSListenPort)

		testNS = "list-" + fuzz.String(5)
		_, err := scaled.CoreFactory.Core().V1().Namespace().Create(&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: testNS, Labels: testResourceLabels},
		})
		MustNotError(err)

		displayNames = map[string]string{}
		for i := 1; i <= 6; i++ {
			name := fmt.Sprintf("vm-%02d", i)
			displayName := fmt.Sprintf("Web %s %02d", testNS, i)
			if i%2 == 0 {
				displayName = fmt.Sprintf("DB %s %02d", testNS, i)
			}
			vm, err := NewDefaultTestVMBuilder(testResourceLabels).Namespace(testNS).Name(name).
				Annotations(map[string]string{displayNameAnnotation: displayName}).
				NetworkInterface(testVMInterfaceName, testVMInterfaceModel, "", builder.NetworkInterfaceTypeMasquerade, "").
				ContainerDisk(testVMContainerDiskName, testVMDefaultDiskBus, false, 1, testVMContainerDiskImageName, testVMContainerDiskImagePullPolicy).
				VM()
			MustNotError(err)
			_, err = vmController.Create(vm)
			MustNotError(err)
			vmNames = append(vmNames, name)
			displayNames[name] = displayName
		}

		// wait until KubeVirt has set the status, and the API server has seen the VMs
		MustFinallyBeTrue(func() bool {
			return slices.Equal(names(nil, testNS), vmNames)
		}, 60*time.Second, 2*time.Second)
		MustFinallyBeTrue(func() bool {
			for _, name := range vmNames {
				vm, err := vmController.Get(testNS, name, metav1.GetOptions{})
				if err != nil || vm.Status.PrintableStatus != kubevirtv1.VirtualMachineStatusStopped {
					return false
				}
			}
			return true
		}, 120*time.Second, 2*time.Second)
	})

	ginkgo.AfterAll(func() {
		if env.IsKeepingTestingResource() {
			return
		}
		err := scaled.CoreFactory.Core().V1().Namespace().Delete(testNS, &metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			GinkgoT().Logf("failed to delete test namespace %s, %v", testNS, err)
		}
	})

	Context("queries that have to give the same result with and without the SQL cache", func() {

		Specify("list in the namespace of the path", func() {
			Expect(names(nil, testNS)).To(Equal(vmNames))
		})

		Specify("list by projectsornamespaces", func() {
			Expect(names(q("projectsornamespaces", testNS, "sort", "metadata.name"), "")).To(Equal(vmNames))
		})

		Specify("list in another namespace does not contain the test VMs", func() {
			Expect(names(nil, "default")).To(BeEmpty())
		})

		Specify("filter by the exact name", func() {
			Expect(names(q("filter", "metadata.name=vm-03"), testNS)).To(Equal([]string{"vm-03"}))
		})

		Specify("sort by name, ascending and descending", func() {
			Expect(names(q("sort", "metadata.name"), testNS)).To(Equal(vmNames))
			Expect(names(q("sort", "-metadata.name"), testNS)).To(Equal(reversed(vmNames)))
		})

		Specify("sort by the display name, ascending and descending", func() {
			Expect(names(q("sort", displayNameField), testNS)).To(Equal(byDisplayName(false)))
			Expect(names(q("sort", "-"+displayNameField), testNS)).To(Equal(byDisplayName(true)))
		})

		Specify("page of a sorted list", func() {
			Expect(names(q("sort", "-metadata.name", "pagesize", "2", "page", "2"), testNS)).
				To(Equal(reversed(vmNames)[2:4]))
			Expect(names(q("sort", "metadata.name", "pagesize", "4", "page", "2"), testNS)).
				To(Equal(vmNames[4:]))
		})

		Specify("filter by the status", func() {
			Expect(names(q("filter", "status.printableStatus="+statusStopped, "sort", "metadata.name"), testNS)).
				To(Equal(vmNames))
			Expect(names(q("filter", "status.printableStatus=Running"), testNS)).To(BeEmpty())
		})

		Specify("a new VM shows up in the list, a deleted VM is gone", func() {
			name := "vm-extra"
			vm, err := NewDefaultTestVMBuilder(testResourceLabels).Namespace(testNS).Name(name).
				NetworkInterface(testVMInterfaceName, testVMInterfaceModel, "", builder.NetworkInterfaceTypeMasquerade, "").
				ContainerDisk(testVMContainerDiskName, testVMDefaultDiskBus, false, 1, testVMContainerDiskImageName, testVMContainerDiskImagePullPolicy).
				VM()
			MustNotError(err)
			_, err = vmController.Create(vm)
			MustNotError(err)
			MustFinallyBeTrue(func() bool {
				return slices.Contains(names(nil, testNS), name)
			})

			MustNotError(vmController.Delete(testNS, name, &metav1.DeleteOptions{}))
			MustFinallyBeTrue(func() bool {
				return !slices.Contains(names(nil, testNS), name)
			})
		})

		Specify("list of namespaces contains the test namespace", func() {
			code, resp, body := list(nsAPI, "", nil)
			Expect(code).To(Equal(http.StatusOK), body)
			Expect(resp.ids()).To(gomega.ContainElement(testNS))
		})
	})

	Context("queries that only work with the SQL cache", func() {

		BeforeEach(func() {
			if !options.SQLCache {
				ginkgo.Skip("needs the SQL cache (default, HARVESTER_SQL_CACHE=true)")
			}
		})

		Specify("filter by the display name, partial match", func() {
			got := names(q("filter", fmt.Sprintf("%s~\"Web %s\"", displayNameField, testNS), "sort", "metadata.name"), testNS)
			Expect(got).To(Equal([]string{"vm-01", "vm-03", "vm-05"}))
		})

		Specify("filter by the display name, exact match", func() {
			got := names(q("filter", fmt.Sprintf("%s=\"DB %s 02\"", displayNameField, testNS)), testNS)
			Expect(got).To(Equal([]string{"vm-02"}))
			// = matches the whole value, a part of it is not found
			got = names(q("filter", fmt.Sprintf("%s=\"DB %s\"", displayNameField, testNS)), testNS)
			Expect(got).To(BeEmpty())
		})

		Specify("filter by the name, = is exact and ~ is partial", func() {
			Expect(names(q("filter", "metadata.name=vm-0"), testNS)).To(BeEmpty())
			Expect(names(q("filter", "metadata.name~vm-0", "sort", "metadata.name"), testNS)).To(Equal(vmNames))
		})

		Specify("filters separated by a comma are an OR", func() {
			got := names(q("filter", fmt.Sprintf("metadata.name=vm-01,%s=\"DB %s 04\"", displayNameField, testNS), "sort", "metadata.name"), testNS)
			Expect(got).To(Equal([]string{"vm-01", "vm-04"}))
		})

		Specify("the count is the total, not the size of the page", func() {
			code, resp, body := list(vmsAPI, testNS, q("pagesize", "2"))
			Expect(code).To(Equal(http.StatusOK), body)
			Expect(resp.Data).To(gomega.HaveLen(2))
			Expect(resp.Count).To(Equal(len(vmNames)))
		})

		Specify("a change of the display name is found right away", func() {
			newName := "Renamed " + testNS
			err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				vm, err := vmController.Get(testNS, "vm-01", metav1.GetOptions{})
				if err != nil {
					return err
				}
				vm = vm.DeepCopy()
				vm.Annotations[displayNameAnnotation] = newName
				_, err = vmController.Update(vm)
				return err
			})
			MustNotError(err)
			displayNames["vm-01"] = newName

			MustFinallyBeTrue(func() bool {
				return slices.Equal(names(q("filter", fmt.Sprintf("%s=\"%s\"", displayNameField, newName)), testNS), []string{"vm-01"})
			})
			Expect(names(q("filter", fmt.Sprintf("%s=\"Web %s 01\"", displayNameField, testNS)), testNS)).To(BeEmpty())
		})

		Specify("a sort or filter on a field that is not indexed is rejected", func() {
			code, _, body := list(vmsAPI, testNS, q("sort", "spec.template.spec.domain.cpu.cores"))
			Expect(code).To(Equal(http.StatusUnprocessableEntity), body)
			code, _, body = list(vmsAPI, testNS, q("filter", "spec.runStrategy=Halted"))
			Expect(code).To(Equal(http.StatusUnprocessableEntity), body)
		})

		Specify("labelSelector, limit and continue are ignored", func() {
			got := names(q("labelSelector", "no-such-label=no-such-value", "sort", "metadata.name"), testNS)
			Expect(got).To(Equal(vmNames))
			got = names(q("limit", "1", "sort", "metadata.name"), testNS)
			Expect(got).To(Equal(vmNames))
			got = names(q("limit", "1", "continue", "not-a-token", "sort", "metadata.name"), testNS)
			Expect(got).To(Equal(vmNames))
		})

		Specify("a filter value with a space has to be quoted", func() {
			code, _, body := list(vmsAPI, testNS, q("filter", displayNameField+"~Web "+testNS))
			Expect(code).NotTo(gomega.Equal(http.StatusOK), body)
		})
	})
})
