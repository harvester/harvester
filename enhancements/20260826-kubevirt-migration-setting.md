# KubeVirt Migration Setting

This enhancement documents the `kubevirt-migration` setting, which exposes the cluster-wide
KubeVirt live migration configuration as a Harvester setting, and defines how the setting
behaves on a cluster that configured the KubeVirt object directly before the setting existed.

## Summary

KubeVirt exposes cluster-wide live migration tunables (parallelism, bandwidth, timeouts,
auto-converge, post-copy, ...) under `spec.configuration.migrations` of the `KubeVirt` object.
Before Harvester v1.7.0 the only way to change them was to edit that object with `kubectl`,
which is unsupported, invisible in the UI, and easy to lose.

The `kubevirt-migration` setting makes those tunables a first-class Harvester setting. The
setting is the source of truth: whatever it holds is written to the KubeVirt object.

On a cluster that already customised the KubeVirt object, the setting **adopts** the existing
configuration the first time it is reconciled, so that the setting reports what the cluster is
actually running rather than silently reverting it to the defaults.

### Related Issues

- https://github.com/harvester/harvester/issues/8581
- https://github.com/harvester/harvester/issues/11517

## Motivation

### Goals

- Configure the KubeVirt cluster-wide migration options through a Harvester setting and the UI.
- Never silently change the live migration behaviour of an existing cluster on upgrade.
- Keep the setting and the KubeVirt object in agreement, so what the UI shows is what the
  cluster does.

### Non-goals

- `nodeDrainTaintKey` is not configurable. Harvester's upgrade scripts depend on the KubeVirt
  default `kubevirt.io/drain`.
- `network` is not configurable here. It is owned by the `vm-migration-network` setting.

## Proposal

### User Stories

#### Story 1: Enable auto-converge

**Before**: An administrator whose write-heavy VMs fail to live-migrate has to
`kubectl edit kubevirt -n harvester-system kubevirt` and set
`spec.configuration.migrations.allowAutoConverge: true`. The change is invisible to Harvester
and to anyone reading the UI.

**After**: The administrator edits the `kubevirt-migration` setting, sets `allowAutoConverge`
to `true`, and saves. The controller writes it to the KubeVirt object.

#### Story 2: Upgrade a cluster that was configured directly

**Before**: A cluster running Harvester v1.6.x has `allowAutoConverge: true` set by hand on the
KubeVirt object. After upgrading, the new `kubevirt-migration` setting shows `false`, because
the setting is created empty and the UI renders the default. The two disagree, and the next
time anybody saves the setting — even to change an unrelated field — auto-converge is silently
turned off.

**After**: On the first reconcile after the upgrade the setting adopts the live configuration,
so it reports `allowAutoConverge: true`. Nothing about the cluster's migration behaviour
changes, and a later edit of an unrelated field no longer discards it.

### User Experience In Detail

The setting is edited from **Advanced → Settings → kubevirt-migration**. Its value is a JSON
object; every field is optional and any field left out falls back to the KubeVirt default.

| Field | Type | Default | Description |
| --- | --- | --- | --- |
| `parallelOutboundMigrationsPerNode` | uint32 | `2` | Concurrent outgoing migrations per node. |
| `parallelMigrationsPerCluster` | uint32 | `5` | Concurrent migrations cluster-wide. |
| `allowAutoConverge` | bool | `false` | Throttle the guest CPU to help a busy VM converge. Can slow the guest down. |
| `bandwidthPerMigration` | quantity | `0` | Bandwidth cap per migration. `0` means unlimited. |
| `completionTimeoutPerGiB` | int64 | `150` | Seconds per GiB of memory before the migration is cancelled. |
| `progressTimeout` | int64 | `150` | Seconds without progress before the migration is cancelled. |
| `unsafeMigrationOverride` | bool | `false` | Migrate even when the compatibility check fails. |
| `allowPostCopy` | bool | `false` | Switch to post-copy when pre-copy cannot converge. A failure during post-copy can crash the VM. |
| `allowWorkloadDisruption` | bool | `false` | Allow a migration to disrupt the workload when it cannot complete otherwise. |
| `disableTLS` | bool | `false` | Disable TLS on the migration connection. |
| `matchSELinuxLevelOnMigration` | bool | `false` | Match the SELinux level on the target node. |

The defaults above are KubeVirt's own compiled-in defaults, so an untouched setting and an
untouched cluster describe the same behaviour.

## Design

### Implementation Overview

`pkg/settings/settings.go` defines the setting and its default. `pkg/webhook/resources/setting`
validates it. `pkg/controller/master/setting/kubevirt_migration.go` reconciles it against the
`kubevirt` object in `harvester-system`.

The reconcile has two paths, chosen by whether the setting has ever been configured. A setting
counts as never configured when it has no `value` **and** no `harvesterhci.io/hash` annotation
— the annotation is written by the setting controller after every successful sync, so its
absence means no sync has ever completed.

**Adopt** (never configured):

1. If the KubeVirt object has no `spec.configuration.migrations`, do nothing. KubeVirt runs on
   its own defaults, which the setting default already mirrors.
2. Otherwise decode the setting default, overlay the fields the KubeVirt object sets, drop
   `nodeDrainTaintKey` and `network`, and write the result to the setting's `value`.
3. Do not write to the KubeVirt object.

Because the adopted value is the default overlaid with the live configuration, the apply path
that runs immediately afterwards is a no-op.

**Apply** (configured, or configured and then cleared):

1. Decode `value`, falling back to `default` when `value` was cleared.
2. Preserve `network` from the KubeVirt object and clear `nodeDrainTaintKey`.
3. Write the result to `spec.configuration.migrations` if it differs.

The setting is listed in `bootstrapSettings` so that the syncer runs even while `value` is
empty; this is what allows adoption to happen without a user touching the setting. It is safe
because the adopt path never writes to the KubeVirt object.

### Validation

The webhook rejects a value that:

- sets `nodeDrainTaintKey` or `network`, since neither is configurable here;
- is not valid JSON for a `MigrationConfiguration`;
- is submitted while a `VirtualMachineInstanceMigration` is in progress. Changing the
  configuration mid-migration would apply to an in-flight migration.

### Upgrade strategy

No upgrade action is required. The setting is created empty by the upgrade, and the first
reconcile adopts whatever is on the KubeVirt object.

Administrators upgrading from a release without this setting should be aware that the setting
becomes authoritative once it holds a value. Editing the KubeVirt object directly after that
point is unsupported: the change will be overwritten the next time the setting is reconciled.

### Test plan

Unit tests in `pkg/controller/master/setting/kubevirt_migration_test.go` cover:

- An unconfigured setting adopts an existing KubeVirt migration configuration and leaves the
  KubeVirt object untouched.
- Adoption drops `nodeDrainTaintKey` and `network`.
- An unconfigured setting against an unconfigured KubeVirt object writes nothing.
- A configured setting is applied to the KubeVirt object, overriding what is there while
  preserving `network`.
- Clearing the value of a previously synced setting resets the KubeVirt object to the default
  rather than re-adopting.

Manual verification on an upgraded cluster:

```bash
# Before the upgrade, on the old release.
kubectl patch kubevirt -n harvester-system kubevirt --type=merge \
  -p '{"spec":{"configuration":{"migrations":{"allowAutoConverge":true}}}}'

# After the upgrade.
kubectl get setting.harvesterhci.io kubevirt-migration -o jsonpath='{.value}' | jq
# -> allowAutoConverge is true

kubectl get kubevirt -n harvester-system kubevirt \
  -o jsonpath='{.spec.configuration.migrations}' | jq
# -> unchanged, allowAutoConverge is still true
```

### Known limitations

- `utilityVolumesTimeout`, present in the KubeVirt API, is not part of the setting default and
  is therefore not editable from the UI. A value already set on the KubeVirt object is carried
  through adoption, but is dropped if the setting is later cleared.
- The setting is not reconciled when the KubeVirt object changes. A direct edit of
  `spec.configuration.migrations` is not reverted until the setting is next updated.
