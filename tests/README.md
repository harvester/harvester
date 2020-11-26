## How to run
  * Configure the appropriate environment variables
    - If there is an existing  cluster, we can locate the KubeConfig to local and `export USE_EXISTING_CLUSTER=true` to use it, default is to create a  local Kind cluster. (Tips) In local test, you can `export KIND_IMAGE_MIRROR="your image mirror url" `to speed up .
    - If there is an existing harvester runtime in the cluster, we can `export USE_EXISTING_RUNTIME=true` to use it, default is to install a harvester runtime to cluster or update the harvester runtime in cluster.
    - If the harvester runtime suuport kvm, we can `export DONT_USE_EMULATION=true` to support it, default is to use qemu software emulation.
    - If we want to review testing cluster, we can `export KEEP_TESTING_CLUSTER=true`  to keep it , default is to delete the local Kind cluster after test finished.
    - If we want to review testing runtime, we can `export KEEP_TESTING_RUNTIME=true`  to keep it, default is to delete the harvester runtime after test finished.
    - If we want to review testing vm, we can `export KEEP_TESTING_VM=true` to keep it, default is to delete each testing vm after each test case finished.
  * Run ./script/test-integration or make test-integration.