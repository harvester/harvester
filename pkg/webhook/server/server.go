package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/rancher/dynamiclistener"
	"github.com/rancher/dynamiclistener/server"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	"github.com/harvester/harvester/pkg/webhook"
	"github.com/harvester/harvester/pkg/webhook/clients"
	"github.com/harvester/harvester/pkg/webhook/config"
	"github.com/harvester/harvester/pkg/webhook/indexeres"
	"github.com/harvester/harvester/pkg/webhook/types"
)

var (
	certName = "harvester-webhook-tls"
	caName   = "harvester-webhook-ca"
	port     = int32(443)

	validationPath      = "/v1/webhook/validation"
	mutationPath        = "/v1/webhook/mutation"
	failPolicyFail      = v1.Fail
	failPolicyIgnore    = v1.Ignore
	sideEffectClassNone = v1.SideEffectClassNone
)

type AdmissionWebhookServer struct {
	context    context.Context
	restConfig *rest.Config
	options    *config.Options
}

func New(ctx context.Context, restConfig *rest.Config, options *config.Options) *AdmissionWebhookServer {
	return &AdmissionWebhookServer{
		context:    ctx,
		restConfig: restConfig,
		options:    options,
	}
}

func (s *AdmissionWebhookServer) ListenAndServe() error {
	clients, err := clients.New(s.context, s.restConfig, s.options.Threadiness)
	if err != nil {
		return err
	}

	indexeres.RegisterIndexers(clients)

	validationHandler, validationResources, err := Validation(clients, s.options)
	if err != nil {
		return err
	}
	mutationHandler, mutationResources, err := Mutation(clients, s.options)
	if err != nil {
		return err
	}

	router := mux.NewRouter()
	router.Handle(validationPath, validationHandler)
	router.Handle(mutationPath, mutationHandler)
	if err := s.listenAndServe(clients, router, validationResources, mutationResources); err != nil {
		return err
	}

	if err := clients.Start(s.context); err != nil {
		return err
	}
	return nil
}

func (s *AdmissionWebhookServer) listenAndServe(clients *clients.Clients, handler http.Handler, validationResources []types.Resource, mutationResources []types.Resource) error {
	apply := clients.Apply.WithDynamicLookup()
	clients.Core.Secret().OnChange(s.context, "secrets", func(key string, secret *corev1.Secret) (*corev1.Secret, error) {
		if secret == nil || secret.Name != caName || secret.Namespace != s.options.Namespace || len(secret.Data[corev1.TLSCertKey]) == 0 {
			return nil, nil
		}
		logrus.Info("Sleeping for 15 seconds then applying webhook config")
		// Sleep here to make sure server is listening and all caches are primed
		time.Sleep(15 * time.Second)

		logrus.Debugf("Building validation rules...")
		validationRules := s.buildRules(validationResources)
		logrus.Debugf("Building mutation rules...")
		mutationRules := s.buildRules(mutationResources)

		validatingWebhookConfiguration := &v1.ValidatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name: webhook.ValidatingWebhookName,
			},
			Webhooks: []v1.ValidatingWebhook{
				{
					Name: "validator.harvesterhci.io",
					ClientConfig: v1.WebhookClientConfig{
						Service: &v1.ServiceReference{
							Namespace: s.options.Namespace,
							Name:      "harvester-webhook",
							Path:      &validationPath,
							Port:      &port,
						},
						CABundle: secret.Data[corev1.TLSCertKey],
					},
					Rules:                   validationRules,
					FailurePolicy:           &failPolicyFail,
					SideEffects:             &sideEffectClassNone,
					AdmissionReviewVersions: []string{"v1", "v1beta1"},
				},
			},
		}

		mutatingWebhookConfiguration := &v1.MutatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name: webhook.MutatingWebhookName,
			},
			Webhooks: []v1.MutatingWebhook{
				{
					Name: "mutator.harvesterhci.io",
					ClientConfig: v1.WebhookClientConfig{
						Service: &v1.ServiceReference{
							Namespace: s.options.Namespace,
							Name:      "harvester-webhook",
							Path:      &mutationPath,
							Port:      &port,
						},
						CABundle: secret.Data[corev1.TLSCertKey],
					},
					Rules:                   mutationRules,
					FailurePolicy:           &failPolicyIgnore,
					SideEffects:             &sideEffectClassNone,
					AdmissionReviewVersions: []string{"v1", "v1beta1"},
				},
			},
		}

		return secret, apply.WithOwner(secret).ApplyObjects(validatingWebhookConfiguration, mutatingWebhookConfiguration)
	})

	tlsName := fmt.Sprintf("harvester-webhook.%s.svc", s.options.Namespace)

	return server.ListenAndServe(s.context, s.options.HTTPSListenPort, 0, handler, &server.ListenOpts{
		Secrets:       clients.Core.Secret(),
		CertNamespace: s.options.Namespace,
		CertName:      certName,
		CAName:        caName,
		TLSListenerConfig: dynamiclistener.Config{
			SANs: []string{
				tlsName,
			},
			FilterCN: dynamiclistener.OnlyAllow(tlsName),
		},
	})
}

func (s *AdmissionWebhookServer) buildRules(resources []types.Resource) []v1.RuleWithOperations {
	rules := []v1.RuleWithOperations{}
	for _, rsc := range resources {
		logrus.Debugf("Add rule for %+v", rsc)
		scope := rsc.Scope
		rules = append(rules, v1.RuleWithOperations{
			Operations: rsc.OperationTypes,
			Rule: v1.Rule{
				APIGroups:   []string{rsc.APIGroup},
				APIVersions: []string{rsc.APIVersion},
				Resources:   rsc.Names,
				Scope:       &scope,
			},
		})
	}

	return rules
}
