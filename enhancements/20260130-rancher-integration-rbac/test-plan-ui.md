# Rancher Integration RBAC - UI Test Plan

This document outlines the UI test plan for the Rancher Integration RBAC enhancement.

The test plan assumes that Harvester is already imported into an external Rancher instance.

Follow the instructions at <https://github.com/harvester/charts/pull/475> to install the `harvester-rbac` Helm chart on Rancher.

## Test Case 1: Cluster Role - assign rancher user with the 'View Virtualization Resources' cluster role

### On Rancher side

Login as the Rancher admin.

Create the new Rancher user:

* Display Name: `virt-viewer`
* username: `virt-viewer`
* Global permission: `Standard User`

Access the Harvester cluster configuration section on the 'Virtualization Management' page.

Under the 'Member Role' section, assign `virt-viewer` the 'View Virtualization Resources' cluster role to the Harvester cluster.

Login to Rancher as `virt-viewer`.

#### Cluster Management

Verify that `virt-viewer` cannot see the list of guest clusters.

Grant `virt-viewer` view access to the guest clusters:

* Login as the Rancher admin (e.g., by using private browsing mode or a different browser)
* Navigate to the guest cluster 'Cluster Configuration' section
* Find the 'Member Role' section and assign `virt-viewer` the 'View Virtualization Resources' cluster role to the guest cluster
* Refresh the page while logged in as `virt-viewer` and verify that they can now see the list of guest clusters, but cannot see the details of the guest cluster or perform any actions on the guest cluster

### On Harvester side

Access the Harvester cluster from the 'Virtualization Management' page as the `virt-viewer` user.

#### Dashboard

Verify that `virt-viewer` can see the 'Capacity' and 'Events' information.

If the monitoring add-on is enabled, they can see 'Cluster Metrics' and 'Virtual Machine Metrics' graphs.

#### Hosts Page

Verify that `virt-viewer` can view the list of hosts and their details, but cannot perform actions such as cordoning the node, enabling maintenance mode, or edit the host config

Known issues:

* `virt-viewer` can see the 'Enable CPU Manager' option

#### Virtual Machines Page

Verify that `virt-viewer` can view the list of VMs and their details, but cannot perform actions such as starting, stopping, or editing the VM.

Known issues:

* `virt-viewer` can see the 'Create Schedule' option even though they won't be able to create the schedule

#### Volumes Page

Verify that `virt-viewer` can view the list of volumes and their details, but cannot perform actions such as creating, editing, or deleting volumes.

#### Images Page

Verify that `virt-viewer` can view the list of images and their details, but cannot perform actions such as creating, editing, or deleting images.

Known issues:

* `virt-viewer` can see the 'Encrypt Image' option even though they won't be able to create the image

#### Projects/Namespaces page

Verify that `virt-viewer` can view the list of projects and namespaces, but cannot perform actions such as creating, editing, or deleting projects/namespaces.

Known issues:

* `virt-viewer` can see the 'Add Members' option on the project details page even though they won't be able to add members

#### Networks Page

Verify that `virt-viewer` can view the list of cluster networks, virtual machine networks, network policies, load balancers and IP pools, but cannot perform actions such as creating, editing, or deleting networks.

#### Backup and Snapshot Page

Verify that `virt-viewer` can view the list of virtual machine backups, snapshots and schedules, but cannot perform actions such as creating, editing, or deleting backups/snapshots.

Known issues:

* `virt-viewer` can see the 'Suspend' option on the backup/snapshot schedule page even though they won't be able to suspend the schedule

#### Monitoring & Logging Page

Verify that `virt-viewer` can view the monitoring and logging configuration, but cannot perform actions such as editing the configuration.

#### RBAC Page

Verify that `virt-viewer` can view the list of cluster members, but cannot perform actions such as creating, editing, or deleting cluster members.

Known issues:

* `virt-viewer` can see the 'Add' option even though they won't be able to add members

#### Advanced Page

Verify that `virt-viewer` can view the 'Advanced' page, but cannot perform actions such as editing the storage class, SSH keys, virtual machine templates, add-ons configuration.

Known issues:

* `virt-viewer` can see the 'Set as Default' option on the storage class page even though they won't be able to change the default storage class
* `virt-viewer` can see the 'Enable/Disable' option on the add-on page  even though they won't be able to enable/disable add-ons

#### Support Page

Verify that `virt-viewer` can view the 'Support' page and generate support bundles.

## Test Case 2: Cluster Role - assign rancher user with the ' Manage Virtualization Resources' cluster role

### On Rancher side

Login as the Rancher admin.

Create the new Rancher user:

* Display Name: `virt-manager`
* Username: `virt-manager`
* Global permission: `Standard User`

Access the Harvester cluster configuration section on the 'Virtualization Management' page.

Under the 'Member Role' section, assign `virt-manager` the 'Manage Virtualization Resources' cluster role to the Harvester cluster.

Login to Rancher as `virt-manager`.

#### Cluster Management

Just like `virt-viewer`, `virt-manager` cannot see the list of guest clusters by default.

Follow the same steps as in Test Case 1 to grant `virt-manager` management access to the guest clusters by assigning them the 'Manage Virtualization Resources' cluster role on the guest cluster configuration page.

### On Harvester side

Verify that `virt-manager` can see everything that `virt-viewer` can with the additional permissions to create, modify and delete existing resources.

## Test Case 3: Project role - assign rancher user with the 'View Virtualization Resources' project role

### On Rancher side

Login as the Rancher admin.

Create the new Rancher user:

* Display Name: `proj-viewer`
* Username: `proj-viewer`
* Global permission: `Standard User`

#### Virtualization Management

Verify that `proj-viewer` cannot see the Harvester cluster.

Grant `proj-viewer` view access to a project in Harvester:

* Login as the Rancher admin (e.g., by using private browsing mode or a different browser)
* Navigate to the 'Projects/Namespaces' page on the Harvester cluster
* Add `proj-viewer` as a project member to your project with the 'View Virtualization Resources' project role

#### Cluster Management

Verify that `proj-viewer` cannot see the guest cluster by default.

Follow the same steps as in Test Case 1 to grant `proj-viewer` management access to the guest clusters by assigning them the 'View Virtualization Resources' cluster role on the guest cluster configuration page.

#### On Harvester side

Verify that `proj-viewer` can see all the resources and namespace-scoped settings in their project on Harvester.

Verify that they can't:

* view resources outside of their project
* modify the resources in their project
* view advanced cluster-scoped settings
* view monitoring metrics

## Test Case 4: Project role - assign rancher user with the 'Manage Virtualization Resources' project role

### On Rancher side

Login as the Rancher admin.

Create the new Rancher user:

* Display Name: `proj-manager`
* Username: `proj-manager`
* Global permission: `Standard User`

#### Virtualization Management

Just like with `proj-viewer`, `proj-manager` cannot see the Harvester cluster by default.

Follow the same steps as in Test Case 3 to grant `proj-manager` management access to a project in Harvester by assigning them the 'Manage Virtualization Resources' project role.

#### Cluster Management

Verify that `proj-manager` cannot see the guest cluster by default.

Follow the same steps as in Test Case 1 to grant `proj-manager` management access to the guest clusters by assigning them the 'Manage Virtualization Resources' cluster role on the guest cluster configuration page.

#### On Harvester side

Verify that `proj-manager` can see and modify all the resources and namespace-scoped settings in their project on Harvester.

Verify that they can't:

* view resources outside of their project
* view advanced cluster-scoped settings
* view monitoring metrics
