# Live Migration

Live migration means moving a virtual machine to a different host without downtime.


>Notes:
>- Live migration is not allowed when the virtual machine is using a management network of bridge interface type.
>- To support live migration, 3 or more hosts in the Harvester cluster are required due to [a known issue](https://github.com/harvester/harvester/issues/798).

#### Starting a migration

1. Go to the **Virtual Machines** page.
1. Find the virtual machine that you want to migrate and select **Vertical &#8942; (... ) > Migrate**.
1. Choose the node that you want to migrate the virtual machine to. Click **Apply**.

#### Aborting a migration

1. Go to the **Virtual Machines** page.
1. Find the virtual machine that is in migrating status and you want to abort the migration. Select **Vertical &#8942; (... ) > Abort Migration**.

#### Migration timeouts

##### Completion timeout

The live migration process will copy virtual machine memory pages and disk blocks to the destination. In some cases, the virtual machine can write to different memory pages / disk blocks at a higher rate than these can be copied, which will prevent the migration process from completing in a reasonable amount of time. Live migration will be aborted if it exceeds the completion timeout which is 800s per GiB of data. For example, a virtual machine with 8 GiB of memory will time out after 6400 seconds.

##### Progress timeout

Live migration will also be aborted when it will be noticed that copying memory doesn't make any progress in 150s.
