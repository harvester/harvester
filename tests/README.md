## Harvester Testing

This path contains the basic testing framework and integration testing cases, the primary focus in here to covering all Harvester APIs with integration testing cases.

### Integration test
The integration test is performed on every pull request and is run by the Drone CI. The script is stored in the `scripts/test-integration` file.

Before the integration test, you must check the AppArmor status to ensure the `usr.sbin.virtqemud` is not enabled.
Or you might see the following error in virt-launcher pods when you test the VM API

```
compute {"component":"virt-launcher","kind":"","level":"error","msg":"Failed to sync vmi","name":"test-grsvz","namespace":"default","pos":"server.go:202","reason":"virError(Code=1, Domain=10, Message='internal error: Failed to start QEMU binary /usr/bin/qemu-system-x86_64 for probing: libvirt:  error : cannot execute binary /usr/bin/qemu-system-x86_64: Operation not permitted\n')","timestamp":"2024-06-26T07:46:15.346012Z","uid":"e4ec1043-d9ce-4246-b672-f554e9a00169"}
```

You can use the following command to check:
```
$ sudo aa-status |grep virtqemud
```

And use the following command to disable:
```
$ sudo aa-disable /etc/apparmor.d/usr.sbin.virtqemud
```

## How to run
  * Configure appropriate environment variables
    - set `KUBECONFIG` path and configure `export USE_EXISTING_CLUSTER=true` to use an existing cluster, default to create a local `Kind` cluster.
        - For the local `Kind` cluster, you can specify `export KIND_IMAGE_MIRROR="http://your-mirror-address:port" ` to speed up pulling images.
    - configure `export SKIP_HARVESTER_INSTALLATION=true` to skip Harvester installation, default to install a new harvester chart or update the existing one.
    - set `export DONT_USE_EMULATION=true` if the harvester runtime supports KVM, default to using `QEMU` software emulation.
    - set `export KEEP_TESTING_CLUSTER=true` if you want to keep the testing cluster, default to delete the local `Kind` cluster.
    - set `export KEEP_HARVESTER_INSTALLATION=true` if you want to keep the harvester installation, default to delete it on exit, this will always be true when `SKIP_HARVESTER_INSTALLATION=true`.
    - set `export KEEP_TESTING_RESOURCE=true` if we want to keep testing resources, default to delete each testing resource after each test case finished.
    - set `export ENABLE_E2E_TESTS=true` if want to run e2e test, the user must have an existing Kubernetes cluster.

* Run `./scripts/test-integration` or `make test-integration`.
    - if you want to use an existing cluster with a pre-installed harvester chart, make sure to config the replicas of `harvester` deployment to be `0`, then run `$ env USE_EXISTING_CLUSTER=true SKIP_HARVESTER_INSTALLATION=true ./scripts/test-integration`. 

