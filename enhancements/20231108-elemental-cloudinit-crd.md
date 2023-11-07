# Kubernetes-native Elemental cloud-init documents

## Summary

It is both an expected and supported behavior for Harvester users to
persist certain configurations to their Harvester nodes. These are currently
done with Elemental cloud-init documents under the `/oem` path on the system.

Currently, Harvester users would have to write these cloud-init documents
and manually push them to each node where they would like to effect
the desired change.

There is a cloud-native tooling gap with regards to this process, however.
Harvester users might prefer to use a GitOps-style approach to configuring
their Harvester clusters, where Kubernetes objects are version-controlled
as YAML and each new revision is applied to the cluster.

This HEP presents a new Custom Resource Definition (CRD) that is surfaced
via the harvester-node-manager, where a Harvester user could apply a "CloudInit"
CR to a Harvester cluster, like so:

```yaml
apiVersion: node.harvesterhci.io/v1beta1
kind: CloudInit
metadata:
  name: ssh-access
spec:
  matchSelector: {}
  filename: 99_ssh.yaml
  contents: |
    stages:
      network:
        - authorized_keys:
            rancher:
              - ssh-ed25519 AAAA...
```

### Related Issues

https://github.com/harvester/harvester/issues/4712

## Motivation

Provide a GitOps-compatible way of persisting configuration changes to
a Harvester cluster.

### Goals

* Elemental cloud-init documents are stored in a CloudInit object spec and
   pulled down onto nodes that match the selector of this CloudInit object.
* CloudInit object's status field describes how it is rolled out
  across the cluster (for example: which nodes does it apply to,
  are any out of sync, etc).

### Non-goals [optional]

* Automatic rollbacks. If a Harvester user wants to "undo" a previously applied
  CloudInit object, they should expect to:

  1. Revise the CloudInit object contents and reboot; or
  2. Adjust the CloudInit object match selector to exclude a node that they
     no longer want it to apply to and reboot; or
  3. Delete the CloudInit object and reboot the nodes it applied to.

* Automatic execution. Harvester users should expect to reboot a
  Harvester node to allow the resulting cloud-init documents to be
  applied at boot.

* Stanza/block-level editing. The CloudInit resource's `.spec.contents`
  field will fully overwrite the file that is referred to in the resource's
  `.spec.filename` field.

## Proposal

### User Stories

Harvester users may customize their Harvester nodes to the fullest extent
that the [Elemental toolkit](
https://rancher.github.io/elemental-toolkit/docs/reference/cloud_init/) allows.

Because of this, there is a very broad and nuanced set of customizations a Harvester
user can describe in a cloud-init-style file.

#### Add/rotate rancher user authorized keys (API)

For example, a user could distribute and/or rotate the `rancher` user's authorized
keys by creating a CloudInit resource like so:

```yaml
apiVersion: node.harvesterhci.io/v1beta1
kind: CloudInit
metadata:
  name: ssh-access
spec:
  matchSelector: {}
  filename: 99_ssh.yaml
  contents: |
    stages:
      network:
        - authorized_keys:
            rancher:
              - ssh-ed25519 AAAA...
```

Then, the user could monitor the ssh-access CloudInit resource to know the status
of its rollout to the Harvester cluster:

```
$ kubectl describe cloudinit ssh-access
# Metadata and Spec omitted for brevity...
Status:
  Rollouts:
    Conditions:
      Last Transition Time:  2023-10-24T21:15:02Z
      Message:               
      Reason:                CloudInitApplicable
      Status:                True
      Type:                  Applicable
      Last Transition Time:  2023-10-24T21:09:28Z
      Message:               Local file checksum is the same as the CloudInit checksum
      Reason:                CloudInitChecksumMatch
      Status:                False
      Type:                  OutOfSync
      Last Transition Time:  2023-10-24T21:09:28Z
      Message:               99_ssh.yaml is present under /oem
      Reason:                CloudInitPresentOnDisk
      Status:                True
      Type:                  Present
    Node Name:               harv1
```

In the case of this single-node cluster, the user can see that the
cloud-init-style file has been copied down to `/oem/99_ssh.yaml` on
node `harv1`.

They can also see that the file is up-to-date since the rollout says
it is not `OutOfSync`.

The user probably already knows that it is `Applicable` to `harv` because
the `ssh-access` CloudInit resource spec says `matchSelector: {}`, which is
a label set that will match every node. That said, if they updated the
`matchSelector` down the line, then the `Applicable` condition may help
them troubleshoot why a node has or does not have a cloud-init-style file
copied down to it.

The user can now reboot the Harvester node so that their cloud-init-style
file may be applied at boot time.

#### Add/rotate rancher user authorized keys (UI/Dashboard)

Alternatively, instead of creating the `ssh-access` resource via kubectl,
there could be a new form added to the Harvester dashboard (perhaps as a
sub-page under the "Hosts" section.) This page could have a table with
a list of the cluster's CloudInit resources.

At the far right of each row for each CloudInit resource, there could be a
3-dot menu much like the Virtual Machines page (and others), where a Harvester
user could click "Edit Config", "Edit YAML", or "delete."

There could also be a button called "Create".

The "Create" and "Edit CloudInit" pages could have a post form that looks
a little something like this (though the "Edit CloudInit page" will have
values pre-populated such as resource name, file name, labels, contents):

1. CloudInit resource name (in the case of the above example, `ssh-access`.)
1. The base filename (in the above example, `99_ssh.yaml`.)
1. A `key` input and a `value` input (like the `Labels` form while editing
a Host config). Another row of `key` and `value` inputs can be added by clicking
the `Add Label` button. In the case of the `ssh-access` example above, these
would be left empty so that the UI passes an empty match selector to match all
nodes (`matchSelector: {}`.)
1. A textbox where the cloud-init YAML contents go.
1. A "save" button (or it could be labeled "Create" or "Update" depending on
   if this is a new CloudInit object or if the user is editing a pre-existing one.)

After clicking "save/create/update", the dashboard could redirect the user to
a more detailed page (much like the one you see for a virtual machine when clicking
on a virtual machine's name on the "Virtual Machines" page of the dashboard.)

This page could present the object's spec and its status fields in a way that
the user can understand the state of the `ssh-access` rollout as described above
with the `kubectl` example.

After the user is satisfied with the CloudInit object they've created,
they can reboot the Harvester node(s) it applies to so that the changes
may take effect.


#### Create CloudInit resource that conflicts with a protected file under `/oem` (API)

A Harvester user may accidentally define a CloudInit resource with a given
filename that conflicts with files that the controller must not modify (see
later on in the document for a description of a webhook to prevent this from
happening.)

For example:

```yaml
apiVersion: node.harvesterhci.io/v1beta1
kind: CloudInit
metadata:
  name: bad-document
spec:
  matchSelector: {}
  filename: grubenv
  contents: |
    Hello, world
```

When attempting to create this resource, or editing a valid CloudInit resource,
the user will receive an error and the request is rejected.

#### Create CloudInit resource that conflicts with a protected file under `/oem` (Dashboard/UI)

As above, the user may find themselves in a similar scenario if they are trying
to create a CloudInit resource with a protected filename or editing a pre-existing
resource's filename to match that of a protected file under `/oem`.

In this case, after clicking "save/update/create", the button may turn red and error
text can indicate that they cannot name their CloudInit resource's filename field after
a protected file that resides under `/oem`.

Alternatively, the UI could offer a richer experience and disable the "save/update/create"
button and present an error message for that field if the user enters a protected filename.
This would just be an extra layer for input validation and potentially improve the user experience.
The backend will still validate this field and reject the request if necessary.

### User Experience In Detail

### API changes

The addition of a CloudInit object to the harvester-node-manager kube apiserver
group.

## Design

### Implementation Overview

A new controller for CloudInit objects will be integrated into the harvester-node-manager.

Users will create a CloudInit object that describes which nodes the file should exist on,
as well as the file's name and its contents.

Here is an example "schema" in Go:

```go
type CloudInitSpec struct {
    // MatchSelector is used to select which Nodes this CloudInit object
    // applies to. For Kubernetes resources, an empty map means "match everything."
    MatchSelector map[string]string `json:"matchSelector"`
    // Filename is the file's base name under `/oem`. For example, `a.yaml` would
    // result in a file appearing as `/oem/a.yaml`. Note that it is the _base name_.
    // Subdirectories are not allowed.
    //
    //   Invalid: `/oem/asdf/a.yaml`
    //     Valid: `/oem/a.yaml`
    Filename      string            `json:"filename"`
    // Contents is an Elemental cloud-init document.
    Contents      string            `json:"contents"`
    // Paused will disable cloud-init file syncing if it is set to true. If it is
    // set to false then the controller will continue syncing updates to the file.
    Paused        bool              `json:"paused"`
}
```

Once a CloudInit object has been created on the cluster, its Status field will describe
how it has been rolled out to each Node of the cluster.

For example:

```
Status:
  Rollouts:
    Conditions:
      Last Transition Time:  2023-10-24T21:15:02Z
      Message:               
      Reason:                CloudInitApplicable
      Status:                True
      Type:                  Applicable
      Last Transition Time:  2023-10-24T21:09:28Z
      Message:               Local file checksum is the same as the CloudInit checksum
      Reason:                CloudInitChecksumMatch
      Status:                False
      Type:                  OutOfSync
      Last Transition Time:  2023-10-24T21:09:28Z
      Message:               99_ssh.yaml is present under /oem
      Reason:                CloudInitPresentOnDisk
      Status:                True
      Type:                  Present
    Node Name:               harv1
```

| Condition | Description | Possible values |
| - | - | - |
| Applicable | Whether or not the Node's labels match the object's match selector | "True" (it should be on the Node), or "False" (it should not be on the Node) |
| Present | Whether or not a file that matches the CloudInit object's filename exists at `/oem/${filename}` | "True" the file is present, or "False" if the file does not exist on the node |
| OutOfSync | Whether or not the file's contents on the Node match the CloudInit object's contents | "True" if the file's contents differ from the intended contents in the CloudInit object, "False" if the contents are the same |

This could help a cluster operator troubleshoot why a CloudInit object is or is not
on a given Node.

harvester-node-manager is deployed as a daemonset, and therefore for each node, it will:

* Install an inotify watch on the `/oem` directory to see if any changes are
  made to files that are managed by a CloudInit object. If so, it will enqueue
  the CloudInit object so the controller can ensure its local contents match the
  corresponding CloudInit object.
* When the harvester-node-manager updates a file under `/oem` it will create a
  Kubernetes Event to indicate that it has done so. This event will describe the
  file path that was modified and the host it was modified on.
* Checksum the cloud-init document contents in the CloudInit spec and store
  that value as a SHA256 hash in an annotation on the CloudInit object
  to easily check for equality between a file under `/oem` and its
  corresponding CloudInit object contents.
* Remove a cloud-init file under `/oem` if it no longer applies to the node (or
  the k8s object itself has been deleted.)

A new webhook is required to protect other files under `/oem`, such as:

* elemental.config
* grubenv
* harvester.config
* install
* 90_custom.yaml
* 99_settings.yaml

This defends against a CloudInit object whose spec's filename could overwrite
a system file:


```yaml
apiVersion: node.harvesterhci.io/v1beta1
kind: CloudInit
metadata:
  name: bad-document
spec:
  matchSelector: {}
  filename: grubenv
  contents: |
    Hello, world
```

### Test plan

#### Webhook

| Description | Expected result |
| - | - |
| Create CloudInit with file name `a.yaml` and any contents | Request is accepted |
| Create CloudInit with file name `/asdf/b.yaml` and any contents | Request is rejected |
| Create CoudInit with file name `harvester.config` and any contents | Request is rejected |
| Create CloudInit with valid file name `a.yaml` and invalid cloud-init syntax | Request is rejected |

#### Controller

| Description | Expected result |
| - | - |
| Create valid CloudInit without the checksum annotation filled in | Controller adds checksum annotation |
| Create valid CloudInit with incorrect checksum annotation filled in | Controller updates checksum annotation |
| Create valid CloudInit object whose Match Selector matches 1+ Harvester nodes | Corresponding nodes have the CloudInit's file under `/oem` with the expected contents |
| Create a valid CloudInit object whose Match Selector does not match 1+ Harvester nodes | The non-matched Harvester nodes do not have the file under `/oem`. |

#### inotify watch

| Description | Expected result |
| - | - |
| Create valid CloudInit and then modify it on disk, then view it again shortly after | The controller has overwritten the local copy with the contents of the CloudInit object |
| Create valid CloudInit and them remove it | The controller restores the file that was removed |

### Upgrade strategy

This is a purely additive change; however, if a Harvester user has already
written out some Elemental cloud-init documents under `/oem` with their own
overrides, they should know that if they create or apply a CloudInit object
with the same filename, their original file will be overwritten.
