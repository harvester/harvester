package api_test

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/onsi/ginkgo/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"

	"github.com/harvester/harvester/pkg/builder"
	"github.com/harvester/harvester/pkg/config"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/tests/framework/env"
	"github.com/harvester/harvester/tests/framework/fuzz"
	"github.com/harvester/harvester/tests/framework/helper"
)

const (
	watchTimeout    = 30 * time.Second
	watchAnnotation = "harvester.test.io/watch"
	watchVMType     = "kubevirt.io.virtualmachine"
)

type watchEvent struct {
	Name string          `json:"name"`
	Data json.RawMessage `json:"data"`
}

type watchedObject struct {
	ID       string `json:"id"`
	Metadata struct {
		Annotations map[string]string `json:"annotations"`
	} `json:"metadata"`
}

// object returns the API object that is sent with the event. It is empty for events
// like resource.start, which carry no object.
func (e watchEvent) object() watchedObject {
	var obj watchedObject
	_ = json.Unmarshal(e.Data, &obj)
	return obj
}

type subscription struct {
	conn   *websocket.Conn
	events chan watchEvent
}

// subscribe opens a websocket to /v1/subscribe and sends the subscription request.
func subscribe(port int, request map[string]interface{}) *subscription {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: true},
	}
	// The default upgrader only accepts an Origin that matches the Host header, including the port.
	header := http.Header{"Origin": []string{fmt.Sprintf("https://localhost:%d", port)}}
	conn, resp, err := dialer.Dial(fmt.Sprintf("wss://localhost:%d/v1/subscribe", port), header)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	MustNotError(err)
	MustNotError(conn.WriteJSON(request))

	sub := &subscription{conn: conn, events: make(chan watchEvent, 1024)}
	go func() {
		defer close(sub.events)
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var event watchEvent
			if json.Unmarshal(message, &event) != nil || event.Name == "ping" {
				continue
			}
			sub.events <- event
		}
	}()
	return sub
}

func (s *subscription) close() {
	_ = s.conn.Close()
}

// waitFor returns the first event that matches. All other events are skipped.
func (s *subscription) waitFor(description string, match func(watchEvent) bool) watchEvent {
	timeout := time.After(watchTimeout)
	var skipped []string
	for {
		select {
		case event, ok := <-s.events:
			if !ok {
				ginkgo.Fail(fmt.Sprintf("subscription closed while waiting for %s, skipped events: %v", description, skipped))
			}
			if match(event) {
				return event
			}
			skipped = append(skipped, event.Name+" "+event.object().ID)
		case <-timeout:
			ginkgo.Fail(fmt.Sprintf("timeout while waiting for %s, skipped events: %v", description, skipped))
		}
	}
}

// quiet returns after no event has arrived for the given time. A watch without a resourceVersion
// may replay the objects that already exist, this waits until that is over.
func (s *subscription) quiet(idle time.Duration) {
	for {
		select {
		case _, ok := <-s.events:
			if !ok {
				return
			}
		case <-time.After(idle):
			return
		}
	}
}

// The specs only look at the events of the VMs they create, so they give the same result with and
// without the SQL cache. Without the cache a new watch replays the existing objects as
// resource.create, with the cache it does not, this is not part of the specs.
var _ = Describe("verify watch of vm events", ginkgo.Ordered, func() {

	var (
		scaled       *config.Scaled
		vmController ctlkubevirtv1.VirtualMachineController
		testNS       string
		events       *subscription
	)

	vmID := func(name string) string {
		return testNS + "/" + name
	}

	createVM := func(name string) {
		vm, err := NewDefaultTestVMBuilder(testResourceLabels).Namespace(testNS).Name(name).
			NetworkInterface(testVMInterfaceName, testVMInterfaceModel, "", builder.NetworkInterfaceTypeMasquerade, "").
			ContainerDisk(testVMContainerDiskName, testVMDefaultDiskBus, false, 1, testVMContainerDiskImageName, testVMContainerDiskImagePullPolicy).
			VM()
		MustNotError(err)
		_, err = vmController.Create(vm)
		MustNotError(err)
	}

	ginkgo.BeforeAll(func() {
		scaled = harvester.Scaled()
		vmController = scaled.VirtFactory.Kubevirt().V1().VirtualMachine()

		testNS = "watch-" + fuzz.String(5)
		_, err := scaled.CoreFactory.Core().V1().Namespace().Create(&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: testNS, Labels: testResourceLabels},
		})
		MustNotError(err)

		// List the type once, so its cache exists before the watch is started.
		vmsAPI := helper.BuildAPIURL("v1", "kubevirt.io.virtualmachines", options.HTTPSListenPort)
		code, body, err := helper.GetResponse(vmsAPI)
		MustNotError(err)
		Expect(code).To(Equal(http.StatusOK), string(body))

		events = subscribe(options.HTTPSListenPort, map[string]interface{}{"resourceType": watchVMType})
		events.waitFor("the start of the subscription", func(e watchEvent) bool {
			return e.Name == "resource.start"
		})
	})

	ginkgo.AfterAll(func() {
		if events != nil {
			events.close()
		}
		if env.IsKeepingTestingResource() {
			return
		}
		err := scaled.CoreFactory.Core().V1().Namespace().Delete(testNS, &metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			GinkgoT().Logf("failed to delete test namespace %s, %v", testNS, err)
		}
	})

	Specify("a new VM is announced as resource.create", func() {
		createVM("vm-watch")
		events.waitFor("resource.create of vm-watch", func(e watchEvent) bool {
			return e.Name == "resource.create" && e.object().ID == vmID("vm-watch")
		})
	})

	Specify("a change of an annotation is announced as resource.change with the new value", func() {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			vm, err := vmController.Get(testNS, "vm-watch", metav1.GetOptions{})
			if err != nil {
				return err
			}
			vm = vm.DeepCopy()
			if vm.Annotations == nil {
				vm.Annotations = map[string]string{}
			}
			vm.Annotations[watchAnnotation] = "changed"
			_, err = vmController.Update(vm)
			return err
		})
		MustNotError(err)

		events.waitFor("resource.change of vm-watch with the new annotation", func(e watchEvent) bool {
			obj := e.object()
			return e.Name == "resource.change" && obj.ID == vmID("vm-watch") &&
				obj.Metadata.Annotations[watchAnnotation] == "changed"
		})
	})

	Specify("a deleted VM is announced as resource.remove", func() {
		MustNotError(vmController.Delete(testNS, "vm-watch", &metav1.DeleteOptions{}))
		events.waitFor("resource.remove of vm-watch", func(e watchEvent) bool {
			return e.Name == "resource.remove" && e.object().ID == vmID("vm-watch")
		})
	})

	Specify("the resource.changes mode sends a notification when a VM is created", func() {
		notifications := subscribe(options.HTTPSListenPort, map[string]interface{}{
			"resourceType": watchVMType,
			"mode":         "resource.changes",
			"debounceMs":   100,
		})
		defer notifications.close()
		notifications.waitFor("the start of the subscription", func(e watchEvent) bool {
			return e.Name == "resource.start"
		})
		notifications.quiet(500 * time.Millisecond)

		createVM("vm-notify")
		notifications.waitFor("a resource.changes notification", func(e watchEvent) bool {
			return e.Name == "resource.changes"
		})
	})
})
