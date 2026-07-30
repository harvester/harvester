package forkliftproxy

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httputil"
	"os"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	authorizationv1client "k8s.io/client-go/kubernetes/typed/authorization/v1"

	"github.com/harvester/harvester/pkg/util"
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
	sar authorizationv1client.SubjectAccessReviewInterface
}

func NewForkliftProxyHandler(sar authorizationv1client.SubjectAccessReviewInterface) *ForkliftProxyHandler {
	return &ForkliftProxyHandler{
		sar: sar,
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

// checkServiceAccess checks if the user can get the forklift-inventory service.
// Users need get access to this service in the forklift namespace to use the proxy API.
func (f *ForkliftProxyHandler) checkServiceAccess(req *http.Request) error {
	userName := req.Header.Get("Impersonate-User")
	groups := req.Header.Values("Impersonate-Extra-Groups")
	logrus.Debugf("Impersonate-User: %s, Impersonate-Extra-Groups: %v", userName, groups)

	allowed, err := util.CheckObjectAccess(req.Context(), util.ResourceAccessCheck{
		SAR:       f.sar,
		Username:  userName,
		Groups:    groups,
		Verb:      util.VerbGet,
		GVR:       util.ServiceGVR,
		Namespace: forkliftInventoryNamespace,
		Name:      forkliftInventoryServiceName,
	})
	if err != nil {
		return err
	}
	if !allowed {
		return fmt.Errorf("user %q is not permitted to get service %s/%s", userName, forkliftInventoryNamespace, forkliftInventoryServiceName)
	}
	return nil
}
