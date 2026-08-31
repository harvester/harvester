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

On a cluster that already customised the KubeVirt object, the upgrade **converts** the existing
configuration into the setting, so that the setting reports what the cluster is actually running
rather than silently reverting it to the defaults the next time it is saved.

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

**After**: The upgrade converts the live configuration into the setting, so it reports
`allowAutoConverge: true`. Nothing about the cluster's migration behaviour changes, and a later
edit of an unrelated field no longer discards it.

The conversion ships in a release later than v1.7.0, so it cannot help a cluster taking the
v1.6.x → v1.7.x hop, which is exactly the hop that introduces the mismatch. See
[Known limitations](#known-limitations) for the workaround.

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

The controller always treats the Harvester setting as the source of truth:

1. Decode `value`, falling back to `default` when `value` is empty.
2. Preserve `network` from the KubeVirt object and clear `nodeDrainTaintKey`.
3. Write the result to `spec.configuration.migrations` if it differs.

Conversion of a pre-existing configuration is handled once, on the upgrade path, rather than in
the controller. This follows the precedent set by `additional-guest-memory-overhead-ratio`
(#6438), which faced the same problem when it started managing a field that users could
previously only set on the KubeVirt object.

`convert_kubevirt_migration_to_harvester_setting` in `package/upgrade/upgrade_manifests.sh` runs
before the Harvester chart is upgraded:

1. Read `spec.configuration.migrations` from the KubeVirt object. If it is absent, stop —
   KubeVirt runs on its own defaults, which the setting default already mirrors.
2. Drop `nodeDrainTaintKey` and `network`, since the webhook rejects a value that carries them.
   If nothing is left, stop.
3. If the setting already holds a value, stop. The user has configured it and it wins.
4. Otherwise write the result into the setting: patch it when it exists (upgrading from a
   release that has the setting but never had a value), create it otherwise (upgrading from a
   release that predates the setting).

The KubeVirt object is never written by the conversion, so the upgrade cannot change the
cluster's live migration behaviour. Once the controller comes up it applies the converted value,
which is what the object already holds, so the first reconcile is a no-op.

Keeping the conversion out of the controller is what lets the controller stay a plain
setting-to-object reconciler. An in-controller adoption would have to write back to the setting
it was invoked for, which races with the `Configured` condition update the setting controller
performs right after every syncer returns.

### Validation

The webhook rejects a value that:

- sets `nodeDrainTaintKey` or `network`, since neither is configurable here;
- is not valid JSON for a `MigrationConfiguration`;
- is submitted while a `VirtualMachineInstanceMigration` is in progress. Changing the
  configuration mid-migration would apply to an in-flight migration.

### Upgrade strategy

The conversion described above runs automatically, both when upgrading from a release that
predates the setting and when upgrading a cluster that is already on a release with the setting
but has never given it a value. It is idempotent and a no-op on a cluster that never customised
the KubeVirt object. It cannot cover the v1.6.x → v1.7.x upgrade itself, see
[Known limitations](#known-limitations).

Administrators should be aware that the setting becomes authoritative once it holds a value.
Editing the KubeVirt object directly after that point is unsupported: the change will be
overwritten the next time the setting is reconciled.

The conversion is skipped, with a warning, if the webhook rejects the write because a VM
migration is in progress. The KubeVirt object keeps its configuration either way, and the
conversion will be retried on the next upgrade. The manual workaround below sets the value
directly.

### Test plan

Unit tests in `pkg/controller/master/setting/kubevirt_migration_test.go` cover the reconcile:

- The value is applied to the KubeVirt object.
- The value replaces the existing configuration but keeps `network`.
- `nodeDrainTaintKey` is cleared.
- A quantity value (`bandwidthPerMigration`) is applied.
- An empty value falls back to the default.
- The KubeVirt object is not updated when it is already in sync.

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

#### The v1.6.x → v1.7.x upgrade is not covered

The setting was introduced in v1.7.0 with no conversion. The conversion added here ships in a
later release, and `upgrade_manifests.sh` is taken from the release being upgraded *to*, so the
v1.6.x → v1.7.x upgrade — the one that creates the mismatch — never runs it.

A cluster that takes that hop is left with the KubeVirt object holding the customised
configuration and the setting holding nothing. The behaviour does not change, and the conversion
still repairs the cluster on its next upgrade to a release that carries it, **provided the
setting has not been saved in the meantime**. If it has, the customisation is already gone and
the conversion correctly declines to touch a setting that now holds a value.

The workaround is to create the setting by hand before upgrading to v1.7.x, which is documented
as the knowledge base article *Preserving a manually configured KubeVirt live migration
configuration*. It runs the same logic as
`convert_kubevirt_migration_to_harvester_setting` as a standalone script, reads the KubeVirt
object and writes only the setting, and is a no-op on a cluster that never customised the
object.

The `kubevirt-migration` entry added to the Harvester documentation links to that article and
warns that the whole value is applied on save.

#### Other limitations

- `utilityVolumesTimeout`, present in the KubeVirt API, is not part of the setting default and
  is therefore not editable from the UI. A value already set on the KubeVirt object is carried
  through the conversion, but is dropped if the setting is later cleared.
- The conversion only runs during an upgrade. A cluster that is already on a release with the
  setting and has the mismatch today keeps it until its next upgrade; the manual workaround
  above sets the value immediately.
- The setting is not reconciled when the KubeVirt object changes. A direct edit of
  `spec.configuration.migrations` is not reverted until the setting is next updated.
