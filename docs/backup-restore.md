# VM Backup & Restore

> Available as of v0.2.0

Users can choose to create VM backups from the **Virtual Machines** page, the VM backup volumes will be stored in the **Backup Target**(an NFS or S3 server) and users can use them to either restore a new VM or replace a existing VM.

> Prerequisite: A backup target must be set up. For more information, see [Backup Target Setup](#backup-target-setup). If the BackupTarget has not been set, you’ll be presented with a prompt message.

## Backup Target Setup
A backup target is an endpoint used to access a backup store in the Harvester. A backup store is an NFS server or S3 compatible server that stores the backups of VM volumes. The backup target can be set at `Settings > backup-target`.

| Parameter | Type |Description |
| ----------- | ----- | ----------- |
| Type | string | Choose S3 or NFS |
| Endpoint | string | EndPoint is a hostname or an IP address. Can be left empty for AWS S3. |
| BucketName | string | Name of the bucket |
| BucketRegion | string | Region of the bucket |
| AccessKeyID | string | AccessKeyID is like a user-id that uniquely identifies your account. |
| SecretAccessKey | string | SecretAccessKey is the password to your account. |
| Certificate | string | Paste the certificate if you want to use a self-signed SSL certificate of your s3 server |
| VirtualHostedStyle | bool | Use virtual-hosted-style access only, e.g., Alibaba Cloud(Aliyun) OSS |

## Create a VM backup
1. Once the backup target is set, go to the `Virtual Machines` page.
1. Click `Take Backup` of the VM actions to create a new VM backup.
1. Set a custom backup name and click `Create` to create a new VM backup.
1. A notification message will be promoted, and users can go to the `Advanced > Backups` page to view all VM backups.
1. The `ReadyToUse` status will be set to true once the Backup is complete.
1. Users can either choose to restore a new VM or replace an existing VM using this backup.

## Restore a new VM using a backup
1. Users can select to restore a new VM using a backup from the `Backups` page.
1. Specify the new VM name and click `Create`.
1. A new VM will be restored using the backup volumes and metadata, and users can access it from the `Virtual Machines` page.

## Replace an Existing VM using a backup
1. Users can replace an existing VM using the backup with the same VM backup target on the `Backups` page.
1. The VM must exist and is required to be in the powered-off status.
1. (Optional) Users can choose to either delete the previous volumes or retain them, default to delete all previous volumes.
1. Click `Create` and the restoring process can be viewed from the `Virtual Machines` page.
