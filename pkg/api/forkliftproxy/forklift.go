package forkliftproxy

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httputil"
	"os"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	satoken                      = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	OriginHeader                 = "Origin"
	forkliftInventoryServiceName = "forklift-inventory"
	forkliftInventoryNamespace   = "forklift"
)

var (
	forkliftInventoryEndpoint = fmt.Sprintf("%s.%s:8443", forkliftInventoryServiceName, forkliftInventoryNamespace)
)

type ForkliftProxyHandler struct {
	config *rest.Config
}

func NewForkliftProxyHandler(config *rest.Config) *ForkliftProxyHandler {
	return &ForkliftProxyHandler{
		config: config,
	}
}

func (f *ForkliftProxyHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	// Extract the path variables map from the request
	vars := mux.Vars(req)
	providerId := vars["providerId"]
	object := vars["object"]

	// on successful auth rancher sets the header Impersonate-Extra-Username
	// we can use this logic to identify if user is not yet authenticated and return 401
	extraUserName := req.Header.Get("Impersonate-Extra-Username")
	if extraUserName == "" {
		http.Error(rw, "Unauthorized: missing authentication headers", http.StatusUnauthorized)
		return
	}
	//validate object to ensure it is one of datastores, networks or vms
	if object != "datastores" && object != "networks" && object != "vms" {
		http.Error(rw, "Invalid object type", http.StatusBadRequest)
		return
	}

	err := f.checkServiceAccess(req)
	if err != nil {
		logrus.Errorf("failed to fetch service with user impersonation: %v", err)
		http.Error(rw, "error verifying access to service: "+err.Error(), http.StatusForbidden)
		return
	}

	director := func(r *http.Request) {
		r.URL.Scheme = "https"
		r.URL.Host = forkliftInventoryEndpoint
		r.URL.Path = fmt.Sprintf("/providers/vsphere/%s/%s", providerId, object)
		r.Header.Set(OriginHeader, fmt.Sprintf("%s://%s", GetOriginScheme(r.URL.Scheme), r.Host))
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	httpProxy := &httputil.ReverseProxy{
		Director:  director,
		Transport: transport,
	}

	token, err := getToken()
	if err != nil {
		http.Error(rw, "Failed to get token for forklift inventory", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	httpProxy.ServeHTTP(rw, req)
}

func getToken() (string, error) {
	tokenBytes, err := os.ReadFile(satoken)
	if err != nil {
		return "", err
	}
	return string(tokenBytes), nil
}

func GetOriginScheme(scheme string) string {
	switch scheme {
	case "ws":
		return "http"
	case "wss":
		return "https"
	default:
		return scheme
	}
}

// checkServiceAccess checks if the user can access the forklift inventory service by attempting to get the service resource in Kubernetes using the user's credentials.
// users will need access to `forklift-inventory` service in `forklift` namespace to pass this check and be able to use the proxy api.
func (f *ForkliftProxyHandler) checkServiceAccess(req *http.Request) error {
	userConfig, err := userImpersonationConfig(f.config, req)
	if err != nil {
		return fmt.Errorf("failed to generate user kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(userConfig)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes clientset: %w", err)
	}

	_, err = clientset.CoreV1().Services(forkliftInventoryNamespace).Get(req.Context(), forkliftInventoryServiceName, metav1.GetOptions{})
	return err
}

func userImpersonationConfig(restConfig *rest.Config, req *http.Request) (*rest.Config, error) {
	userConfig := rest.CopyConfig(restConfig)
	userName := req.Header.Get("Impersonate-User")
	groups := req.Header.Values("Impersonate-Extra-Groups")
	logrus.Debugf("Impersonate-User: %s, Impersonate-Extra-Groups: %v", userName, groups)
	if userName == "" {
		return nil, fmt.Errorf("missing required authentication headers")
	}

	logrus.Debugf("Impersonating user for api call: %s, groups: %s", userName, groups)
	userConfig.Impersonate = rest.ImpersonationConfig{
		UserName: userName,
		Groups:   groups,
	}
	return userConfig, nil
}
