# Bare-metal Cluster Container and Multi-cluster Management Support

Make the bare-metal Harvester support container workloads and multi-clusters management.

## Summary

This proposal aims to improve Harvester's usability and reduce resource consumption by providing container workload management and multi-cluster management through the bare-metal Harvester cluster.

The current [Rancher integration in Harvester](https://docs.harvesterhci.io/v1.1/rancher/rancher-integration) requires deploying Rancher separately, which can be complicated and resource-intensive. By enabling container workloads and multi-cluster management through Harvester's built-in Rancher will reduce overhead and improve product usability.

### Related Issues

https://github.com/harvester/harvester/issues/2679


## Motivation

### Goals

- Provide support for managing bare-metal container workloads.
- Provide multi-cluster management(MCM) support through the bare-metal Harvester cluster.
- Reduce the footprint/overhead of the existing Harvester-Rancher integration method.
- Support authentication and authorization on the bare-metal Harvester cluster.


### Non-goals

- Support built-in Rancher manual upgrade without Harvester's new release.
- Support importing and managing Harvester with multiple Ranchers.
- Fleet management support for the Harvester cluster(will be considered as a seperated new feature).
- Automatically force shutdown user containers that block upgrades. Instead, we may consider draining the node, leaving the user to resolve blocking issues manually, such as volume or pod eviction.
- User container workloads might block upgrade. We may force shutdown user containers? (or drain the node only, user need to resolve blocking cases manually, e.g, volume or pod can’t be evicted)
- Backup and restore Rancher (see https://ranchermanager.docs.rancher.com/v2.6/pages-for-subheaders/backup-restore-and-disaster-recovery)
- Container GPU pass-through support (moved to v1.3.0)

## Proposal

- Provide support for managing bare-metal container workloads
    - When the `rancher-manager-support` setting is enabled, users can view and manage their container workloads management via the explorer UI which will appears in the hamburger menu.
    - If the MCM setting is not enabled, users can still access the explorer UI through the existing link on the Harvester support page.
- Provide multi-cluster management(MCM) support through the bare-metal Harvester cluster.
    - Users can enable or disable multi-cluster view through the `rancher-manager-support` setting, either before or after installation.
    - Once enabled, the `Continuous Delivery`, `Cluster Management`, `Users& Authentication` manu will appear.
    - After enabling multi-cluster feature, users cannot import the current Harvester cluster into another Rancher or MCM-enabled Harvester cluster.
    - The multi-cluster view in Harvester cluster will be slightly different from the native Rancher server. e.g., Users can not disable the MCM and Harvester Integration feature flag in the global setting page.
- Reduce the footprint and overhead of the existing Harvester Rancher integration method.
    - Users don't need to deploy additional Rancher, which saves CPU, memory, and disk resources for constrained conditions like edge deployment.
- Support authentication and authorization on the bare-metal Harvester cluster.
    - We can leverage the Rancher's existing authentication and authorization mechanism, but users need to turn on the `rancher-manager-support` setting first.


### User Stories

#### Story 1

As a user, I want to manage my container workloads through the Harvester bare-metal cluster, without having to deploy Rancher separately. This will save me time and resources.


#### Story 2

As a user, I want to use the current bare-metal Harvester cluster for multi-cluster management. Therefore, I can easily spin up a guest rke2 cluster or import any other existing k8s cluster into the Harvester cluster.

#### Story 3

As a user, I want to enable authentication and authorization on the bare-metal Harvester cluster, making it convenient for me to manage resources with proper permissions in an edge case.

### User Experience In Detail

1. Users can enable/disable multi-cluster view through the `rancher-manager-support` setting, either before or after installation.
   - Once enabled, the `Expolorer UI`, `Continuous Delivery`, `Cluster Management`, `Users& Authentication` page will be shown in the hamburger menu.

  ![Harvester-Dashboard-memu.png](./20230316-baremetal-container-mcm-support/harvester-dashboard.png)

3. Users can enable the local cluster authentication and authorization on the `Users & Authentication` page when the `multi-clsuter` configuration is set to true
   - When the `multi-cluster` configuration is set to false, the `Users & Authentication` page will be hidden and the configured auth provider will be disabled automatically, but users can still login with the local auth(i.e., using username and password).
   - If the cluster is imported into another Rancher server, the users & RBAC will not be migrated and only the local auth will be available.

  ![harvester-auth.png](./20230316-baremetal-container-mcm-support/harvester-auth.png)

### API changes

1. A new setting `rancher-manager-support` will be added to the Harvester setting page and can be configured via the [Harvester configuration file](https://docs.harvesterhci.io/v1.1/install/harvester-configuration):
```json
{
  "rancher-manager-support": true # default to false, enable/disable the Rancher multi-cluster view.
}
```

2. Users can visit the local Harvester cluster via `/v1/harvester` API endpoint, and the remote Harvester cluster via `/v1/management.cattle.io.clusters/<cluster-id>/v1/harvester` API endpoint.
   - the local `/v1/harvester` API endpoint is only available starting from the v1.2 after enabling the multi-cluster-management feature in the Rancher server config.


## Design

### Single Harvester UI

The Single UI will remain mostly unchanged, and users can still access the embedded Rancher explorer UI when the 'rancher-manager-support' setting is disabled.

### Multi-cluster UI Design Change

1. Add the `Harvester Dashboard` menu at the top of the `Home` menu to allow users to visit the local Harvester dashboard page easily(local Harvester mode only).
![Harvester-Dashboard-memu.png](./20230316-baremetal-container-mcm-support/harvester-dashboard.png)
2. Virtualization Management pages
    - Change the `version` column value on the `Virtualization Management` page to the Harvester version.(require backend support)
    ![harvester-version.png](./20230316-baremetal-container-mcm-support/harvester-version.png)
    - Add a `Set as login page` option to the header of the Harvester dashboard page.
5. Show the Harvester clusters in the `Cluster Management` & `Continuous Delivery` & `Explore cluster` list. In the `EXPLORE CLUSTER` list, if it is a Harvester cluster, we need to replace it with the Harvester icon.
6. Global setting(local Harvester mode only):
   - Sorting out global setting and harvester settings, remove/hide duplicate settings(require backend research)
   - Lock the `harvester` and `multi-cluster-management` in the `Feature Flags` list like rke2 (can not be disabled)
7. Home page
   - Allow users to jump from the home page to the Harvester dashboard by adding a Harvester icon to the right of the name. Clicking on the icon will take the user to the Harvester Dashboard page.
   - Show all clusters (including Harvester) on the home page.
   - The current local cluster's provider is shown as Custom, but it should be shown as Harvester, including the cluster management table page. See the screenshot below:
   ![home-icon.png](./20230316-baremetal-container-mcm-support/home-icon.png)
8. Cluster Management page:
   - Add the `Explore` button to the end of the local cluster.
9. Explorer UI of local Harvester cluster
   - Disable/hide the Rancher upgrade button on the `Install Apps` page.
   ![rancher-upgrade-button.png](./20230316-baremetal-container-mcm-support/rancher-upgrade-button.png)
   - Disable edit/upgrade/delete Harvester managed system service (e.g., monitoring, logging, Harvester...)
    ![harvester-service.png](./20230316-baremetal-container-mcm-support/harvester-service.png)
   - In the `Cluster Tools` page, we should disable operations that could affect Harvester, such as edit/delete monitor, longhorn, and logging.
    ![cluster-tools.png](./20230316-baremetal-container-mcm-support/cluster-tools.png)
   - If the `local` cluster is a harvester cluster, we need to hide resources prefixed with `harvesterhci.io.*`, and `kubevirt.io.*`
    ![harvester-crd.png](./20230316-baremetal-container-mcm-support/harvester-crd.png)

### Implementation Overview

1. Turn on the `multi-cluster-management` feature in the embedded Rancher server config.
3. Add a new setting `rancher-manager-support` to the Harvester setting page that will allow configuration in both installation and post-installation stage.
4. Customize the Rancher UI to show/hide the explorer UI and the multi-cluster management UI based on the `rancher-manager-support` setting. 
    - Showing both the local Harvester cluster and imported Harvester clusters on the explorer and continues delivery clusters.
    - Hide overlapped and undesired configurations(e.g., disable Harvester and MCM feature) in both Rancher's global setting page and the local Harvester cluster setting page.
    - Replace the `Home` with `Harvester Dashboard` by default going to the local Harvester Dashboard to provide consistent user experience of single and multi-cluster views.


### Test plan

- Users should be able to enable/disable the container workloads management feature via the setting page, and the explorer UI will be shown in the hamburger menu.
- Users should be able to enable/disable multi-cluster view through the `rancher-manager-support` setting, either before or after installation, and the related UI will be shown in the hamburger menu.
- Users should be able to visit both the local Harvester cluster and imported Harvester/k8s clusters explorer UI.
- Users should be able to visit both the local Harvester cluster and imported Harvester clusters virtualization management UI.
- Users can spin-up guest k8s clusters in the local bare-metal cluster through Harvester node driver.
- Users can enable authentication and authorization on the bare-metal Harvester cluster, and the [multi-tenancy](https://ranchermanager.docs.rancher.com/v2.6/pages-for-subheaders/authentication-config) feature on the local Harvester cluster should be identical to a seperated upstream Rancher deployment.

### Upgrade strategy

- According to Rancher's [doc](https://ranchermanager.docs.rancher.com/getting-started/installation-and-upgrade/installation-references/feature-flags), the `multi-cluster-management` is used for multi-cluster provisioning and management of Kubernetes clusters. This feature flag can only be set at install time and not changed afterwards. Therefore, we will need to check the upgrade path since this is disabled by default in the Harvester's embedded Rancher.
- For new installed Harvester this is not a problem, since the `multi-cluster-management` is enabled by default.
- For existing Harvester cluster that is already import by another Rancher, you can only enable the `multi-cluster-management` feature by editing the Rancher server config, and then restart the Rancher server pod.