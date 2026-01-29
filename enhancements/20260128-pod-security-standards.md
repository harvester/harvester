# Harvester PSS Support

## Summary

This enhancement allows Harvester cluster admins to control pod security standards at a cluster level, while allowing fine grained control to allow different pod security standards at per namespace level. By default this setting would be disabled. When enabled by the cluster administrator the `baseline` pod security standard will be applied to all namespaces, except a predefined list of harvester system component namespaces. In addition cluster admins can also apply `privileged` and `restricted` pod security standards on user namespaces or even `whitelist` namespaces which will allow namespace to be excluded from being managed by harvester.


### Related Issues

https://github.com/harvester/harvester/issues/8196

## Motivation

### Goals

Harvester has experimental support for baremetal workloads. With increased adoption a number of users are looking to leverage the underlying Harvester cluster for baremetal workloads. By default rke2 applies a cluster wide `privileged` pod security standard. While users can tweak the pod security standard by applying `AdmissionConfiguration` to the apiserver as documented [here](https://harvesterhci.io/kb/2025/05/08/using-pod-security-standard). This approach requires a restart of the apiserver to ensure the new `AdmissionConfiguration` is applied.

By moving the pod security standard application via a harvester setting to reconcile against namespace labels, we can allow pod security standards to be controlled dynamically by cluster admins, without needing to restart the apiserver.


## Proposal

This is where we get down to the nitty gritty of what the proposal actually is.

### User Stories

#### Story 1: Block users from running privileged workloads in a multi-tenanted cluster
As a cluster admin, I want to block users from running workloads that can access the underlying host nodes.

#### Story 2
As a cluster admin, I only want specific trusted workloads to have privileged access, such as 3rd party csi to run on the cluster.

### User Experience In Detail

The cluster admin can restrict workload behaviour by editing the setting `cluster-pod-security-standard` and enabling the setting.

The setting application can be further fine via changes to `restrictedNamespacesList` to allow certain namespaces to run with a `restricted` pod security standard.

Workloads needing privileged access can be whitelisted via `privilegedNamespacesList`.

Cluster admins can skip namespaces from being managed by the setting by adding them to the `whitelistedNamespacesList`. This can be used if admins wish to test various pod security standards before applying to via the setting.

```json
{"enabled":true,"whitelistedNamespacesList":"default", "restrictedNamespacesList": "demo,restricted-ns","privilegedNamespacesList":"demo2,privileged-ns"}
```

### API changes

A new setting `cluster-pod-security-standard` will be introduced to Harvester

## Design

### Implementation Overview

The current [implementation](https://github.com/harvester/harvester/pull/8302) introduces a setting `cluster-pod-security-standard`

The setting accepts a json string input allowing cluster admins to apply the `baseline` pod security standard at the cluster level, and if needed fine tune specific namespaces via the `restrictedNamespacesList`, `whitelistedNamespacesList` and `privilegedNamespacesList` settings.

```
{"enabled":true,"whitelistedNamespacesList":"default", "restrictedNamespacesList": "demo,restricted-ns","privilegedNamespacesList":"demo2,privileged-ns"}
```

The setting handler in harvester will reconcile all namespaces and apply the correct pod security standard to them. When the setting is `disabled` the handler will ensure the pod security standards are removed from all namespaces, which will result in the cluster returning back to `privileged` pod security standard.

In addition there is additional logic in the harvester webhook to ensure that pod security standard labels can only be managed by the harvester controller. Any user attempts to amend pod security policy unless the namespace is part of the `whitelistedNamespaceList` will be blocked.

The settings handler will skip a predefined list of namespaces from pod security application. These are various rancher / harvester namespaces needed for effective operation of core harvester capability

The namespaces are derived from: https://ranchermanager.docs.rancher.com/how-to-guides/new-user-guides/authentication-permissions-and-global-configuration/psa-config-templates#exempting-required-rancher-namespaces

In addition following harvester specific namespaces are added:
* harvester-system
* harvester-public
* rancher-vcluster
* cattle-dashboards
* fleet-local
* local
* forklift


### Test plan

#### Verify pod security standards are being applied
* Enable `cluster-pod-security-standard` setting and verify `default` namespace has the `baseline` pod security standard applied.
* Create a namespace `demo`, and verify `baseline` pod security standard is applied.
* Create a namespace `demo1`, and add it to `restrictedNamespacesList` and verify the `restricted` pod security standard is applied to the namespace.
* Create a namespace `demo2`, and apply it to `privilegedNamespacesList` and verify the `privileged` pod security standard is applied to the namespace.
* Create a namespace `demo3`, and apply it to `whitelistedNamespacesList` and verify no pod security standard is applied to the namespace.
* Attempt to edit the pod security setting label on `demo1` and `demo2` namespace should fail.
* Attempt to apply custom pod security setting label to `demo3` namespace should be successful.
* Disable the `cluster-pod-security-standard` and validate `default`, `demo`, `demo1`, `demo2` namespace no longer have the pod security standard applied.
* Verify `demo3` which was part of `whitelistedNamespaceList` still has its custom pod security standard applied.

#### Upgrade a cluster with pod security standards enabled
* Enable `cluster-pod-security-standard` setting and verify `default` namespace has the `baseline` pod security standard applied.
* Trigger an upgrade from a new install of harvester containing the setting to another master head build
* Upgrade should be successful, as the system namespaces needing privileged access are already whitelisted

### Upgrade strategy

When upgrading an existing cluster to a version that includes this enhancement, the `cluster-pod-security-standard` setting will be created with `enabled` set to `false`, so cluster behavior remains unchanged until an administrator explicitly enables and configures the setting.

## Note [optional]

Details of actual restrictions enforced by various pod security standards are available [here](https://kubernetes.io/docs/concepts/security/pod-security-standards/)
