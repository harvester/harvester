#!/bin/bash -x

remove_bundledeployment_from_harvester_validator_validatingwebhookconfigurations() {
    echo "Removing BundleDeployment from harvester-validatorValidatingWebhookConfigurations"
    # find and remove "BundleDeployment" from the operations list of the first rule in harvester-validator ValidatingWebhookConfiguration
    kubectl get validatingwebhookconfigurations.admissionregistration.k8s.io harvester-validator -o json | jq 'del(.webhooks[0].rules[] | select(.resources[] == "bundledeployments"))' | kubectl apply -f -
}

remove_bundledeployment_from_harvester_validator_validatingwebhookconfigurations
