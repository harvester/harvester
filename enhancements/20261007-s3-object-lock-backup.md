# Support S3 Object Lock for Harvester Backup

## Summary

This enhancement adds S3 Object Lock (WORM — Write Once, Read Many) support for Harvester Virtual Machine backups. S3 Object Lock prevents backup manifests and raw volume data from being overwritten or deleted before a configured retention date.

Harvester splits backups into two parts: VM configuration metadata managed directly by the Harvester controller, and volume snapshot data blocks managed by Longhorn. Supporting S3 Object Lock requires adding retention settings and admission delete checks to Harvester, while updating Longhorn (`github.com/longhorn/backupstore` and `longhorn-manager`) to write S3 retention headers, isolate transient lock files, and extend retention on deduplicated blocks during incremental backups.

### Related Issues

- https://jira.suse.com/browse/SURE-10224

## Motivation

Backups stored in standard S3 buckets are vulnerable to accidental deletion, compromised administrative credentials, and ransomware. While compliance standards (such as SEC Rule 17a-4 and HIPAA) require immutable recovery copies, Harvester currently cannot enforce retention periods on S3 targets.

Today, configuring an S3 bucket with bucket-level default retention breaks backup operations. Longhorn's backupstore writes temporary concurrency lock files (`locks/<volume>/lock-*.lck`) and immediately tries to delete them, accumulating locked versions or failing with `403 AccessDenied`. Furthermore, when a user deletes a `VirtualMachineBackup` CR in Harvester, standard S3 `DeleteObject` calls only place a delete marker, hiding the object in Harvester while locked underlying versions remain in S3.

This enhancement introduces explicit Object Lock support with verification against the target bucket, writes retention headers on metadata and data blocks, and blocks premature deletion at the Kubernetes admission layer.

### Goals

1. Configure S3 Object Lock parameters (`enabled`, `mode`, `defaultRetentionDays`) in the Harvester `backup-target` setting.
2. Verify that the target S3 bucket has Object Lock and versioning enabled before accepting configuration changes.
3. Write S3 Object Lock retention headers on Harvester VM metadata manifests (`.cfg`) and Longhorn volume backup blocks (`.blk`).
4. Allow per-backup and scheduled retention overrides in `VirtualMachineBackup` and `ScheduleVMBackup` YAML specs.
5. Reject `DELETE` requests in the Harvester admission webhook while a backup is still within its retention period.
6. Support both `GOVERNANCE` and `COMPLIANCE` retention modes on AWS S3 and SeaweedFS (`weed s3`).
7. Keep base VM image (`VirtualMachineImage`) exports unlocked by default so catalog management is not blocked by long backup retention policies.

### Non-goals

1. Harvester will not create buckets or toggle Object Lock on the S3 backend; bucket creation and Object Lock initialization remain external prerequisites.
2. NFS targets do not support S3 Object Lock APIs and are excluded.
3. Decreasing retention duration or deleting backups locked under `COMPLIANCE` mode before expiration is unsupported.

## Proposal

### User Stories

#### Story 1: Immutable backups for regulatory compliance
An administrator configures `COMPLIANCE` mode with 30-day retention. If a compromised credential or unauthorized user issues a delete command against the Harvester API or directly in S3, the objects cannot be deleted or overwritten until the 30 days elapse.

#### Story 2: Protected backups with administrative bypass
An administrator uses `GOVERNANCE` mode by default. Routine operators cannot delete active backups from the UI or CLI. In storage emergencies, an administrator with the `s3:BypassGovernanceRetention` permission can prune older recovery points.

#### Story 3: Per-backup retention overrides
Scheduled daily backups retain for 14 days, while quarterly accounting backups retain for 365 days. The operator overrides the retention period in the `VirtualMachineBackup` spec without altering the cluster-wide target default.

#### Story 4: Clear feedback when deletion is blocked
An operator runs `kubectl delete vmbackup <name>` on an active backup. Harvester's admission webhook immediately denies the request with HTTP 403, stating the retention mode and exact expiration timestamp, rather than leaving the CR hung in `Terminating`.

### User Experience In Detail

#### 1. Configuring the Backup Target
In the Harvester UI or via the `backup-target` setting:
- Enable **Object Lock**.
- Select the default mode: `GOVERNANCE` (default) or `COMPLIANCE`.
- Enter the default retention period in days (e.g., `30`).
- Harvester checks the bucket using `s3:GetBucketObjectLockConfiguration`. If Object Lock or versioning is disabled on the bucket, Harvester rejects the change with an error.

#### 2. Creating Backups with Retention
Backups inherit the target's default retention mode and duration unless overridden in YAML:
```yaml
apiVersion: harvesterhci.io/v1beta1
kind: VirtualMachineBackup
metadata:
  name: prod-db-q3
  namespace: default
spec:
  source:
    apiGroup: kubevirt.io
    kind: VirtualMachine
    name: prod-db
  retention:
    mode: COMPLIANCE
    days: 90
```
`ScheduleVMBackup` resources accept the same `retention` block, applying it to all backups created by the schedule.

#### 3. Deleting Backups
When an operator deletes a backup:
- The admission webhook checks `status.objectLock`.
- If `time.Now() < retainUntilDate`:
  - `COMPLIANCE` mode: Rejects deletion with HTTP 403:
    `VirtualMachineBackup "default/prod-db-q3" is locked under COMPLIANCE mode until 2027-01-05T00:00:00Z and cannot be deleted.`
  - `GOVERNANCE` mode: Rejects deletion unless the request carries an administrative bypass annotation.

### API changes

#### 1. Harvester Settings (`pkg/settings/settings_helper.go`)
Extend `BackupTarget`:
```go
type ObjectLockConfig struct {
    Enabled              bool   `json:"enabled"`
    Mode                 string `json:"mode"` // "GOVERNANCE" or "COMPLIANCE"
    DefaultRetentionDays int64  `json:"defaultRetentionDays"`
}

type BackupTarget struct {
    Type                     TargetType        `json:"type"`
    Endpoint                 string            `json:"endpoint"`
    AccessKeyID              string            `json:"accessKeyId"`
    SecretAccessKey          string            `json:"secretAccessKey"`
    BucketName               string            `json:"bucketName"`
    BucketRegion             string            `json:"bucketRegion"`
    Cert                     string            `json:"cert"`
    VirtualHostedStyle       bool              `json:"virtualHostedStyle"`
    RefreshIntervalInSeconds int64             `json:"refreshIntervalInSeconds"`
    ObjectLock               *ObjectLockConfig `json:"objectLock,omitempty"`
}
```

#### 2. Harvester CRD `VirtualMachineBackup` (`pkg/apis/harvesterhci.io/v1beta1/backup.go`)
Add retention fields to `VirtualMachineBackupSpec` and `VirtualMachineBackupStatus`:
```go
type RetentionMode string

const (
    RetentionModeGovernance RetentionMode = "GOVERNANCE"
    RetentionModeCompliance RetentionMode = "COMPLIANCE"
)

type RetentionSpec struct {
    // Mode defines the S3 Object Lock mode (GOVERNANCE or COMPLIANCE).
    // Defaults to the backup target's configured mode.
    // +optional
    // +kubebuilder:validation:Enum=GOVERNANCE;COMPLIANCE
    Mode RetentionMode `json:"mode,omitempty"`

    // Days defines the number of days this backup will remain immutable.
    // Defaults to the backup target's configured defaultRetentionDays.
    // +optional
    // +kubebuilder:validation:Minimum=1
    Days int64 `json:"days,omitempty"`
}

type ObjectLockStatus struct {
    // Locked indicates whether the backup is protected by S3 Object Lock.
    Locked bool `json:"locked"`

    // Mode indicates the active retention mode.
    Mode RetentionMode `json:"mode,omitempty"`

    // RetainUntilDate marks the point in time until which the backup cannot be deleted.
    RetainUntilDate *metav1.Time `json:"retainUntilDate,omitempty"`
}

type VirtualMachineBackupSpec struct {
    // ... existing fields ...
    // +optional
    Retention *RetentionSpec `json:"retention,omitempty"`
}

type VirtualMachineBackupStatus struct {
    // ... existing fields ...
    // +optional
    ObjectLock *ObjectLockStatus `json:"objectLock,omitempty"`
}
```

#### 3. Harvester CRD `ScheduleVMBackup` (`pkg/apis/harvesterhci.io/v1beta1/schedulebackup.go`)
`ScheduleVMBackupSpec` already embeds `VMBackupSpec VirtualMachineBackupSpec` (`spec.vmbackup`). Because `Retention *RetentionSpec` is added directly to `VirtualMachineBackupSpec`, scheduled backups support `spec.vmbackup.retention` without modifying `ScheduleVMBackupSpec`.

## Design

### Implementation Overview

Harvester VM backups consist of two components:
1. **Longhorn Block Engine**: Snapshots volume data and writes deduplicated 2MB compressed blocks (`blocks/<volume>/<hash>.blk`) and manifests (`backup_<id>.cfg`) to S3.
2. **Harvester Backup Controller**: Writes VM configuration metadata directly to S3 (`harvester/vmbackups/<namespace>/<vmbackup-name>.cfg`).

```
+-----------------------------------------------------------------------------------+
| Harvester Cluster                                                                 |
|                                                                                   |
|  [ VirtualMachineBackup CR ]                                                      |
|         |                                                                         |
|         +--> Harvester Backup Controller                                          |
|         |       |                                                                 |
|         |       +--> Upload VM Metadata (.cfg) + Retention Headers                |
|         |                                                                         |
|         +--> VolumeSnapshot (Longhorn CSI)                                        |
|                 |                                                                 |
|                 +--> Longhorn Manager / Engine                                    |
|                         |                                                         |
|                         +--> Upload Deduplicated Volume Blocks (.blk)             |
|                              & Backup Manifests (.cfg) + Retention Headers        |
+-----------------------------------------------------------------------------------+
                                    |
                                    v
+-----------------------------------------------------------------------------------+
| S3 Object Storage (AWS S3 / SeaweedFS)                                            |
|                                                                                   |
|  harvester/vmbackups/<ns>/<backup>.cfg   [Locked until RetainUntilDate]           |
|  volumes/.../backups/backup_<id>.cfg     [Locked until RetainUntilDate]           |
|  blocks/<volume>/<checksum>.blk         [Locked until Max(Referencing Backups)]  |
|                                                                                   |
|  locks/<volume>/lock-*.lck               [UNLOCKED / Dedicated Prefix]           |
+-----------------------------------------------------------------------------------+
```

### Upstream Longhorn Requirements

Longhorn handles volume block storage and must be updated before Harvester can support Object Lock:

#### 1. `github.com/longhorn/backupstore`
- **S3 Retention Headers**: Add `ObjectLockMode` and `ObjectLockRetainUntilDate` to `s3.PutObjectInput` in `backupstore/s3/s3_service.go`, or expose `PutObjectRetention`.
- **Driver Interface Extension (`backupstore/driver.go`)**: `BackupStoreDriver` currently exposes `Write(dst string, rs io.ReadSeeker) error` without metadata options. Extend the interface (e.g. `WriteWithOptions` or retention parameters) so callers like Harvester's metadata uploader can set object retention during upload.
- **Transient Lock File Isolation**: `backupstore/lock.go` creates temporary lock files (`locks/<volume>/lock-*.lck`) during backup, restore, and garbage collection, and deletes them immediately upon completion. If bucket-level default retention is active, deleting these files either creates delete markers while locked versions accumulate, or fails on strict S3 appliances. Lock files must use a dedicated unlocked prefix or S3 conditional writes.
- **Mutable `volume.cfg`**: Longhorn rewrites `volume.cfg` on every incremental backup. S3 Object Lock requires bucket versioning, so overwriting `volume.cfg` creates new versioned objects. Longhorn must manage `volume.cfg` versions without breaking volume queries.
- **Incremental Deduplication & Shared Block Retention Extension**: Longhorn shares 2MB data blocks across incremental backups of the same volume. When an incremental backup references existing shared blocks, Longhorn must extend the retention date on those blocks using `PutObjectRetention` to match the expiration date of the new backup.
- **Garbage Collection Awareness**: Longhorn's block garbage collector (`cleanupBlocks`) must verify retention dates before issuing delete calls, avoiding `403 AccessDenied` errors on locked blocks.

#### 2. `longhorn-manager`
- Add retention parameters to `BackupTargetSpec` and `BackupSpec`.
- Prevent recurring cleanup jobs from attempting to delete unexpired backups.

### Harvester Implementation Details

#### 1. Setting Validator Webhook (`pkg/webhook/resources/setting/validator.go`)
When `objectLock.enabled` is true in `backup-target`:
- Calls `s3:GetBucketObjectLockConfiguration`.
- Confirms Object Lock and versioning are enabled on the bucket.
- Rejects the setting change if the bucket is not configured for Object Lock.

#### 2. Backup Target Controller (`pkg/controller/master/backup/backup_target.go`)
- Syncs Object Lock parameters to Longhorn's `BackupTarget` CR in namespace `longhorn-system`.
- Stores credentials and retention configuration in the Longhorn secret.

#### 3. Harvester VM Backup Controller (`pkg/controller/master/backup/backup.go`)
- Calculates `retainUntilDate = creationTimestamp + retention.days`.
- Passes retention parameters to the Longhorn `Backup` / `VolumeSnapshot`.
- Calls `PutObject` with retention headers when uploading `harvester/vmbackups/<namespace>/<vmbackup-name>.cfg`.
- Updates `status.objectLock` with `locked: true`, `mode`, and `retainUntilDate`.

#### 4. Admission Webhook Deletion Interceptor (`pkg/webhook/resources/virtualmachinebackup/validator.go`)
- Intercepts `DELETE` requests for `VirtualMachineBackup`.
- If `status.objectLock.locked` is true and `time.Now().Before(status.objectLock.retainUntilDate)`:
  - In `COMPLIANCE` mode: Rejects deletion with HTTP 403.
  - In `GOVERNANCE` mode: Rejects deletion unless an administrative bypass annotation is present.

### State of the Art & Base Image Decoupling

In enterprise backup architectures, WORM protection applies to point-in-time recovery points, not base operating system templates:
- **Restores are Read Operations**: S3 Object Lock prevents writes and deletes, but allows `s3:GetObject`. Restores read locked blocks without issue.
- **Golden Image Decoupling**: Harvester allows exporting `VirtualMachineImage` records to the backup target. Locking golden images for 365 days would prevent administrators from retiring obsolete base images. Therefore, `VirtualMachineBackup` objects are locked according to retention policy, while `VirtualMachineImage` backups remain unlocked by default.

### Backend Compatibility

| Backend | Implementation | Governance Mode | Compliance Mode |
|---|---|---|---|
| **AWS S3** | Native S3 Object Lock API (`x-amz-object-lock-*`) | Supported | Supported |
| **SeaweedFS** | S3 API layer (`weed s3`) | Supported | Supported |

### Test plan

#### Integration & Automated Tests
1. **Target Validation**:
   - Verify that configuring an S3 bucket without Object Lock fails validation.
   - Verify that configuring an Object-Lock-enabled bucket succeeds and syncs to Longhorn.
2. **Backup Creation & Immutability**:
   - Create a `VirtualMachineBackup` with 7 days retention in `GOVERNANCE` mode.
   - Verify `status.objectLock` shows `locked: true` and the correct `retainUntilDate`.
   - Verify `harvester/vmbackups/<ns>/<name>.cfg` and Longhorn volume blocks (`.blk`) in S3 carry retention headers.
3. **Deletion Guardrails**:
   - Verify `kubectl delete vmbackup <name>` fails with HTTP 403 while the lock is active.
   - Test deletion in `GOVERNANCE` mode with bypass credentials.
4. **Restore Validation**:
   - Restore a VM from a locked backup; verify the restore completes successfully.
5. **Incremental Retention Extension**:
   - Create Backup 1 with 10-day retention.
   - Create Backup 2 five days later with 10-day retention, sharing deduplicated blocks.
   - Verify the retention period of shared blocks is extended to match Backup 2.
6. **Backend Matrix**:
   - Validate end-to-end functionality on AWS S3 and SeaweedFS (`weed s3`).

### Upgrade strategy

- Existing clusters retain their current `backup-target` configuration with `objectLock.enabled: false`.
- Backups created prior to enabling Object Lock remain unlocked.
- If Object Lock is disabled on the target, existing locked backups remain protected in S3 until their retention expires, while new backups are written unlocked.

## Note [optional]

### S3 IAM Permissions
The credentials configured in Harvester must have the following permissions:
- `s3:GetBucketObjectLockConfiguration`
- `s3:PutObject`
- `s3:GetObject`
- `s3:GetObjectVersion`
- `s3:PutObjectRetention`
- `s3:GetObjectRetention`
- `s3:BypassGovernanceRetention` (only if Governance bypass is needed)
