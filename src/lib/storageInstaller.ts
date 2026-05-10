import { StorageConfig } from '../types';

export interface StorageInstallationResult {
  success: boolean;
  manifests: string[];
  commands: string[];
  errors: string[];
  status: string;
}

export class StorageInstaller {
  static async installStorage(config: StorageConfig): Promise<StorageInstallationResult> {
    const result: StorageInstallationResult = {
      success: false,
      manifests: [],
      commands: [],
      errors: [],
      status: 'Starting installation...'
    };

    try {
      switch (config.storageType) {
        case 'ceph':
          return await this.installCeph(config);
        case 'nfs':
          return await this.installNFS(config);
        case 'smb':
          return await this.installSMB(config);
        case 'nvme':
          return await this.installNVMe(config);
        case 'rdma':
          return await this.installRDMA(config);
        case 'zfs':
          return await this.installZFS(config);
        case 'iscsi':
          return await this.installISCSI(config);
        case 'glusterfs':
          return await this.installGlusterFS(config);
        case 'longhorn':
          return await this.installLonghorn(config);
        case 'openebs':
          return await this.installOpenEBS(config);
        case 'portworx':
          return await this.installPortworx(config);
        case 'local':
          return await this.installLocal(config);
        default:
          throw new Error(`Unsupported storage type: ${config.storageType}`);
      }
    } catch (error) {
      result.errors.push(error instanceof Error ? error.message : 'Unknown error');
      result.status = 'Installation failed';
      return result;
    }
  }

  private static async installCeph(config: StorageConfig): Promise<StorageInstallationResult> {
    const result: StorageInstallationResult = {
      success: false,
      manifests: [],
      commands: [],
      errors: [],
      status: 'Installing Ceph cluster...'
    };

    // Generate Rook Ceph operator installation
    const operatorManifest = this.generateCephOperatorManifest(config);
    result.manifests.push(operatorManifest);

    // Generate Ceph cluster manifest
    if (config.cephInstallManagers || config.cephInstallOSDs || config.cephInstallMDS || config.cephInstallRGW) {
      const clusterManifest = this.generateCephClusterManifest(config);
      result.manifests.push(clusterManifest);
    }

    // Generate storage class
    const storageClassManifest = this.generateCephStorageClassManifest(config);
    result.manifests.push(storageClassManifest);

    // Installation commands
    result.commands = [
      'kubectl create namespace rook-ceph',
      'kubectl apply -f ceph-operator.yaml',
      'kubectl apply -f ceph-cluster.yaml',
      'kubectl apply -f ceph-storageclass.yaml'
    ];

    result.success = true;
    result.status = 'Ceph installation manifests generated successfully';
    return result;
  }

  private static async installNFS(config: StorageConfig): Promise<StorageInstallationResult> {
    const result: StorageInstallationResult = {
      success: false,
      manifests: [],
      commands: [],
      errors: [],
      status: 'Installing NFS storage...'
    };

    // Generate NFS provisioner deployment
    const nfsManifest = this.generateNFSProvisionerManifest(config);
    result.manifests.push(nfsManifest);

    // Installation commands
    result.commands = [
      `helm repo add nfs-subdir-external-provisioner https://kubernetes-sigs.github.io/nfs-subdir-external-provisioner/`,
      `helm install nfs-provisioner nfs-subdir-external-provisioner/nfs-subdir-external-provisioner \\
        --set nfs.server=${config.nfsServer} \\
        --set nfs.path=${config.nfsPath} \\
        --set storageClass.name=${config.storageClass}`
    ];

    result.success = true;
    result.status = 'NFS installation manifests generated successfully';
    return result;
  }

  private static async installSMB(config: StorageConfig): Promise<StorageInstallationResult> {
    const result: StorageInstallationResult = {
      success: false,
      manifests: [],
      commands: [],
      errors: [],
      status: 'Installing SMB storage...'
    };

    // Generate SMB CSI driver
    const smbManifest = this.generateSMBProvisionerManifest(config);
    result.manifests.push(smbManifest);

    // Installation commands
    result.commands = [
      'kubectl create namespace csi-smb',
      'kubectl apply -f smb-csi-driver.yaml',
      'kubectl create secret generic smb-secret --from-literal=username=${config.smbUsername} --from-literal=password=${config.smbPassword}'
    ];

    result.success = true;
    result.status = 'SMB installation manifests generated successfully';
    return result;
  }

  private static async installNVMe(config: StorageConfig): Promise<StorageInstallationResult> {
    const result: StorageInstallationResult = {
      success: false,
      manifests: [],
      commands: [],
      errors: [],
      status: 'Installing NVMe-oF storage...'
    };

    // Generate NVMe CSI driver
    const nvmeManifest = this.generateNVMeProvisionerManifest(config);
    result.manifests.push(nvmeManifest);

    result.commands = [
      'kubectl create namespace nvme-csi',
      'kubectl apply -f nvme-csi-driver.yaml'
    ];

    result.success = true;
    result.status = 'NVMe-oF installation manifests generated successfully';
    return result;
  }

  private static async installRDMA(config: StorageConfig): Promise<StorageInstallationResult> {
    const result: StorageInstallationResult = {
      success: false,
      manifests: [],
      commands: [],
      errors: [],
      status: 'Installing RDMA storage...'
    };

    // Generate RDMA CSI driver
    const rdmaManifest = this.generateRDMAProvisionerManifest(config);
    result.manifests.push(rdmaManifest);

    result.commands = [
      'kubectl create namespace rdma-csi',
      'kubectl apply -f rdma-csi-driver.yaml'
    ];

    result.success = true;
    result.status = 'RDMA installation manifests generated successfully';
    return result;
  }

  private static async installZFS(config: StorageConfig): Promise<StorageInstallationResult> {
    const result: StorageInstallationResult = {
      success: false,
      manifests: [],
      commands: [],
      errors: [],
      status: 'Installing ZFS storage...'
    };

    // Generate ZFS operator
    const zfsManifest = this.generateZFSOperatorManifest(config);
    result.manifests.push(zfsManifest);

    result.commands = [
      'kubectl create namespace zfs',
      'kubectl apply -f zfs-operator.yaml',
      'kubectl apply -f zfs-pool.yaml'
    ];

    result.success = true;
    result.status = 'ZFS installation manifests generated successfully';
    return result;
  }

  private static async installISCSI(config: StorageConfig): Promise<StorageInstallationResult> {
    const result: StorageInstallationResult = {
      success: false,
      manifests: [],
      commands: [],
      errors: [],
      status: 'Installing iSCSI storage...'
    };

    // Generate iSCSI provisioner
    const iscsiManifest = this.generateISCSIProvisionerManifest(config);
    result.manifests.push(iscsiManifest);

    result.commands = [
      'kubectl create namespace iscsi',
      'kubectl apply -f iscsi-provisioner.yaml'
    ];

    result.success = true;
    result.status = 'iSCSI installation manifests generated successfully';
    return result;
  }

  private static async installGlusterFS(config: StorageConfig): Promise<StorageInstallationResult> {
    const result: StorageInstallationResult = {
      success: false,
      manifests: [],
      commands: [],
      errors: [],
      status: 'Installing GlusterFS storage...'
    };

    // Generate GlusterFS provisioner
    const glusterManifest = this.generateGlusterFSProvisionerManifest(config);
    result.manifests.push(glusterManifest);

    result.commands = [
      'kubectl create namespace glusterfs',
      'kubectl apply -f glusterfs-provisioner.yaml'
    ];

    result.success = true;
    result.status = 'GlusterFS installation manifests generated successfully';
    return result;
  }

  private static async installLonghorn(config: StorageConfig): Promise<StorageInstallationResult> {
    const result: StorageInstallationResult = {
      success: false,
      manifests: [],
      commands: [],
      errors: [],
      status: 'Installing Longhorn storage...'
    };

    // Generate Longhorn operator
    const longhornManifest = this.generateLonghornOperatorManifest(config);
    result.manifests.push(longhornManifest);

    result.commands = [
      'kubectl create namespace longhorn-system',
      'kubectl apply -f longhorn-operator.yaml',
      'kubectl apply -f longhorn-ui-ingress.yaml'
    ];

    result.success = true;
    result.status = 'Longhorn installation manifests generated successfully';
    return result;
  }

  private static async installOpenEBS(config: StorageConfig): Promise<StorageInstallationResult> {
    const result: StorageInstallationResult = {
      success: false,
      manifests: [],
      commands: [],
      errors: [],
      status: 'Installing OpenEBS storage...'
    };

    // Generate OpenEBS operator
    const openebsManifest = this.generateOpenEBSOperatorManifest(config);
    result.manifests.push(openebsManifest);

    result.commands = [
      'kubectl create namespace openebs',
      'kubectl apply -f openebs-operator.yaml'
    ];

    result.success = true;
    result.status = 'OpenEBS installation manifests generated successfully';
    return result;
  }

  private static async installPortworx(config: StorageConfig): Promise<StorageInstallationResult> {
    const result: StorageInstallationResult = {
      success: false,
      manifests: [],
      commands: [],
      errors: [],
      status: 'Installing Portworx storage...'
    };

    // Generate Portworx operator
    const portworxManifest = this.generatePortworxOperatorManifest(config);
    result.manifests.push(portworxManifest);

    result.commands = [
      'kubectl create namespace portworx',
      'kubectl apply -f portworx-operator.yaml'
    ];

    result.success = true;
    result.status = 'Portworx installation manifests generated successfully';
    return result;
  }

  private static async installLocal(config: StorageConfig): Promise<StorageInstallationResult> {
    const result: StorageInstallationResult = {
      success: false,
      manifests: [],
      commands: [],
      errors: [],
      status: 'Installing local storage...'
    };

    const localManifest = this.generateLocalProvisionerManifest(config);
    result.manifests.push(localManifest);
    result.commands = [
      'kubectl create namespace local-storage',
      'kubectl apply -f local-storage-provisioner.yaml'
    ];

    result.success = true;
    result.status = 'Local storage installation manifests generated successfully';
    return result;
  }

  // Manifest generation methods
  private static generateCephOperatorManifest(config: StorageConfig): string {
    return `apiVersion: v1
kind: Namespace
metadata:
  name: rook-ceph
---
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  targetNamespaces:
  - rook-ceph
---
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  channel: stable
  name: rook-ceph
  source: community-operators
  sourceNamespace: openshift-marketplace`;
  }

  private static generateCephClusterManifest(config: StorageConfig): string {
    const monitors = config.cephManagerCount || 3;
    const osds = config.cephOSDCount || 3;

    return `apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: ${config.cephClusterName || 'rook-ceph'}
  namespace: rook-ceph
spec:
  cephVersion:
    image: quay.io/ceph/ceph:v18.2.0
    allowUnsupported: false
  dataDirHostPath: /var/lib/rook
  skipUpgradeChecks: false
  continueUpgradeAfterChecksEvenIfNotHealthy: false
  mon:
    count: ${monitors}
    allowMultiplePerNode: false
  mgr:
    count: ${config.cephManagerCount || 2}
    allowMultiplePerNode: false
  dashboard:
    enabled: true
    ssl: true
  monitoring:
    enabled: false
    rulesNamespace: rook-ceph
  network:
    connections:
      encryption:
        enabled: false
      compression:
        enabled: false
    providers:
      cephfs: ${config.cephInstallMDS ? 'true' : 'false'}
      rbd: true
      rgw: ${config.cephInstallRGW ? 'true' : 'false'}
  crashCollector:
    disable: false
  placement:
    all:
      nodeAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
          nodeSelectorTerms:
          - matchExpressions:
            - key: kubernetes.io/os
              operator: In
              values:
              - linux
      podAntiAffinity:
        preferredDuringSchedulingIgnoredDuringExecution:
        - weight: 100
          podAffinityTerm:
            labelSelector:
              matchExpressions:
              - key: app
                operator: In
                values:
                - rook-ceph-osd
            topologyKey: kubernetes.io/hostname
  annotations:
  labels:
  resources:
  storage:
    useAllNodes: true
    useAllDevices: true
    deviceFilter: ${config.cephDevices?.join(',') || ''}
    config:
      metadataDevice:
      databaseSizeMB: "1024"
      journalSizeMB: "1024"
  disruptionManagement:
    managePodBudgets: false
    osdMaintenanceTimeout: 30
    pgHealthCheckTimeout: 0
  healthCheck:
    daemonHealth:
      mon:
        disabled: false
        interval: 45s
      osd:
        disabled: false
        interval: 60s
      status:
        disabled: false
        interval: 60s`;
  }

  private static generateCephStorageClassManifest(config: StorageConfig): string {
    return `apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ${config.storageClass}
provisioner: rook-ceph.rbd.csi.ceph.com
parameters:
  clusterID: ${config.cephClusterName || 'rook-ceph'}
  pool: ${config.cephPoolName || 'kubernetes'}
  imageFormat: "2"
  imageFeatures: layering
  csi.storage.k8s.io/provisioner-secret-name: csi-rbd-secret
  csi.storage.k8s.io/provisioner-secret-namespace: rook-ceph
  csi.storage.k8s.io/controller-expand-secret-name: csi-rbd-secret
  csi.storage.k8s.io/controller-expand-secret-namespace: rook-ceph
  csi.storage.k8s.io/node-stage-secret-name: csi-rbd-secret
  csi.storage.k8s.io/node-stage-secret-namespace: rook-ceph
  csi.storage.k8s.io/fstype: ext4
reclaimPolicy: Delete
allowVolumeExpansion: true
volumeBindingMode: Immediate`;
  }

  private static generateNFSProvisionerManifest(config: StorageConfig): string {
    return `apiVersion: v1
kind: ServiceAccount
metadata:
  name: nfs-client-provisioner
  namespace: default
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: nfs-client-provisioner-runner
rules:
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["get", "list", "watch", "create", "delete"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["storageclasses"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["create", "update", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: run-nfs-client-provisioner
subjects:
  - kind: ServiceAccount
    name: nfs-client-provisioner
    namespace: default
roleRef:
  kind: ClusterRole
  name: nfs-client-provisioner-runner
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: leader-locking-nfs-client-provisioner
  namespace: default
rules:
  - apiGroups: [""]
    resources: ["endpoints"]
    verbs: ["get", "list", "watch", "create", "update", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: leader-locking-nfs-client-provisioner
  namespace: default
subjects:
  - kind: ServiceAccount
    name: nfs-client-provisioner
    namespace: default
roleRef:
  kind: Role
  name: leader-locking-nfs-client-provisioner
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nfs-client-provisioner
  labels:
    app: nfs-client-provisioner
  namespace: default
spec:
  replicas: 1
  strategy:
    type: Recreate
  selector:
    matchLabels:
      app: nfs-client-provisioner
  template:
    metadata:
      labels:
        app: nfs-client-provisioner
    spec:
      serviceAccountName: nfs-client-provisioner
      containers:
        - name: nfs-client-provisioner
          image: k8s.gcr.io/sig-storage/nfs-subdir-external-provisioner:v4.0.2
          volumeMounts:
            - name: nfs-client-root
              mountPath: /persistentvolumes
          env:
            - name: PROVISIONER_NAME
              value: k8s-sigs.io/nfs-subdir-external-provisioner
            - name: NFS_SERVER
              value: ${config.nfsServer}
            - name: NFS_PATH
              value: ${config.nfsPath}
      volumes:
        - name: nfs-client-root
          nfs:
            server: ${config.nfsServer}
            path: ${config.nfsPath}
---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ${config.storageClass}
provisioner: k8s-sigs.io/nfs-subdir-external-provisioner
parameters:
  archiveOnDelete: "false"`;
  }

  private static generateSMBProvisionerManifest(config: StorageConfig): string {
    return `apiVersion: v1
kind: Secret
metadata:
  name: smb-secret
  namespace: csi-smb
type: Opaque
data:
  username: ${btoa(config.smbUsername || '')}
  password: ${btoa(config.smbPassword || '')}
---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ${config.storageClass}
provisioner: smb.csi.k8s.io
parameters:
  source: "//${config.smbServer}/${config.smbShare}"
  csi.storage.k8s.io/provisioner-secret-name: smb-secret
  csi.storage.k8s.io/provisioner-secret-namespace: csi-smb
  csi.storage.k8s.io/node-stage-secret-name: smb-secret
  csi.storage.k8s.io/node-stage-secret-namespace: csi-smb
reclaimPolicy: Delete
volumeBindingMode: Immediate
mountOptions:
  - dir_mode=0777
  - file_mode=0777
  ${config.smbMultichannel ? '- mfsymlinks' : ''}`;
  }

  private static generateNVMeProvisionerManifest(config: StorageConfig): string {
    return `apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ${config.storageClass}
provisioner: nvme.csi.k8s.io
parameters:
  nqn: "${config.nvmeSubsystemNQN}"
  transport: "${config.nvmeTransport || 'tcp'}"
  target: "${config.nvmeTargetIP}:${config.nvmeTargetPort || 4420}"
  namespace: "1"
reclaimPolicy: Delete
volumeBindingMode: Immediate`;
  }

  private static generateRDMAProvisionerManifest(config: StorageConfig): string {
    return `apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ${config.storageClass}
provisioner: rdma.csi.k8s.io
parameters:
  device: "${config.rdmaDevice || 'ib0'}"
  port: "${config.rdmaPort || 4420}"
  maxQueueDepth: "${config.rdmaMaxQueueDepth || 32}"
reclaimPolicy: Delete
volumeBindingMode: Immediate`;
  }

  private static generateZFSOperatorManifest(config: StorageConfig): string {
    return `apiVersion: apps/v1
kind: Deployment
metadata:
  name: zfs-operator
  namespace: zfs
spec:
  replicas: 1
  selector:
    matchLabels:
      app: zfs-operator
  template:
    metadata:
      labels:
        app: zfs-operator
    spec:
      containers:
      - name: operator
        image: quay.io/openebs/zfs-localpv:latest
        env:
        - name: OPENEBS_IO_ZFS_DRIVER
          value: "zfs"
        - name: OPENEBS_IO_INSTALL_DEFAULT_CSI_DRIVER
          value: "true"
---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ${config.storageClass}
provisioner: zfs.csi.openebs.io
parameters:
  poolname: "${config.zfsPoolName}"
  compression: "${config.zfsCompression ? 'on' : 'off'}"
  dedup: "${config.zfsDedup ? 'on' : 'off'}"
reclaimPolicy: Delete
volumeBindingMode: Immediate`;
  }

  private static generateLocalProvisionerManifest(config: StorageConfig): string {
    return `apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: local-volume-provisioner
  namespace: local-storage
spec:
  selector:
    matchLabels:
      app: local-volume-provisioner
  template:
    metadata:
      labels:
        app: local-volume-provisioner
    spec:
      containers:
      - name: provisioner
        image: quay.io/external_storage/local-volume-provisioner:v2.5.0
        env:
        - name: MY_NODE_NAME
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
        volumeMounts:
        - name: local-storage
          mountPath: /mnt/local-storage
      volumes:
      - name: local-storage
        hostPath:
          path: /mnt/local-storage
---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ${config.storageClass}
provisioner: kubernetes.io/no-provisioner
volumeBindingMode: WaitForFirstConsumer
reclaimPolicy: Delete`;
  }

  private static generateISCSIProvisionerManifest(config: StorageConfig): string {
    return `apiVersion: v1
kind: ConfigMap
metadata:
  name: iscsi-config
  namespace: iscsi
data:
  config.yaml: |
    default-target-portal: "${config.iscsiTarget || '192.168.1.100'}:3260"
    default-iqn: "${config.iscsiIqn || 'iqn.2020-01.com.example:target'}"
    default-lun: "${config.iscsiLun || 0}"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: iscsi-provisioner
  namespace: iscsi
spec:
  replicas: 1
  selector:
    matchLabels:
      app: iscsi-provisioner
  template:
    metadata:
      labels:
        app: iscsi-provisioner
    spec:
      containers:
      - name: iscsi-provisioner
        image: quay.io/external_storage/iscsi-provisioner:latest
        env:
        - name: PROVISIONER_NAME
          value: iscsi.csi.k8s.io
        volumeMounts:
        - name: config
          mountPath: /etc/config
      volumes:
      - name: config
        configMap:
          name: iscsi-config
---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ${config.storageClass}
provisioner: iscsi.csi.k8s.io
parameters:
  targetPortal: "${config.iscsiTarget || '192.168.1.100'}:3260"
  iqn: "${config.iscsiIqn || 'iqn.2020-01.com.example:target'}"
  lun: "${config.iscsiLun || 0}"
  fsType: ext4
reclaimPolicy: Delete
volumeBindingMode: Immediate`;
  }

  private static generateGlusterFSProvisionerManifest(config: StorageConfig): string {
    const servers = config.glusterfsServers || ['192.168.1.100', '192.168.1.101'];

    return `apiVersion: apps/v1
kind: Deployment
metadata:
  name: glusterfs-provisioner
  namespace: glusterfs
spec:
  replicas: 1
  selector:
    matchLabels:
      app: glusterfs-provisioner
  template:
    metadata:
      labels:
        app: glusterfs-provisioner
    spec:
      containers:
      - name: glusterfs-provisioner
        image: gluster/glusterfs-client:latest
        env:
        - name: PROVISIONER_NAME
          value: glusterfs.csi.k8s.io
        - name: GLUSTERFS_SERVERS
          value: "${servers.join(',')}"
        - name: GLUSTERFS_VOLUME
          value: "${config.glusterfsVolume || 'vol1'}"
        - name: GLUSTERFS_TRANSPORT
          value: "${config.glusterfsTransport || 'tcp'}"
---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ${config.storageClass}
provisioner: glusterfs.csi.k8s.io
parameters:
  servers: "${servers.join(',')}"
  volume: "${config.glusterfsVolume || 'vol1'}"
  transport: "${config.glusterfsTransport || 'tcp'}"
reclaimPolicy: Delete
volumeBindingMode: Immediate`;
  }

  private static generateLonghornOperatorManifest(config: StorageConfig): string {
    return `apiVersion: v1
kind: Namespace
metadata:
  name: longhorn-system
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: longhorn-service-account
  namespace: longhorn-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: longhorn-role
rules:
- apiGroups: ["*"]
  resources: ["*"]
  verbs: ["*"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: longhorn-bind
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: longhorn-role
subjects:
- kind: ServiceAccount
  name: longhorn-service-account
  namespace: longhorn-system
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: longhorn-manager
  namespace: longhorn-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: longhorn-manager
  template:
    metadata:
      labels:
        app: longhorn-manager
    spec:
      serviceAccountName: longhorn-service-account
      containers:
      - name: longhorn-manager
        image: longhornio/longhorn-manager:v1.4.0
        ports:
        - containerPort: 9500
        env:
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: POD_IP
          valueFrom:
            fieldRef:
              fieldPath: status.podIP
        volumeMounts:
        - name: dev
          mountPath: /host/dev/
        - name: proc
          mountPath: /host/proc/
        - name: varrun
          mountPath: /var/run/
      volumes:
      - name: dev
        hostPath:
          path: /dev/
      - name: proc
        hostPath:
          path: /proc/
      - name: varrun
        hostPath:
          path: /var/run/
---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ${config.storageClass}
provisioner: driver.longhorn.io
parameters:
  numberOfReplicas: "${config.longhornReplicaCount || 3}"
  dataLocality: "${config.longhornDataLocality || 'disabled'}"
  backingImage: "${config.longhornBackingImage || ''}"
reclaimPolicy: Delete
volumeBindingMode: Immediate`;
  }

  private static generateOpenEBSOperatorManifest(config: StorageConfig): string {
    return `apiVersion: v1
kind: Namespace
metadata:
  name: openebs
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: openebs-operator
  namespace: openebs
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: openebs-operator
rules:
- apiGroups: ["*"]
  resources: ["*"]
  verbs: ["*"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: openebs-operator
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: openebs-operator
subjects:
- kind: ServiceAccount
  name: openebs-operator
  namespace: openebs
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: openebs-provisioner
  namespace: openebs
spec:
  replicas: 1
  selector:
    matchLabels:
      app: openebs-provisioner
  template:
    metadata:
      labels:
        app: openebs-provisioner
    spec:
      serviceAccountName: openebs-operator
      containers:
      - name: openebs-provisioner
        image: openebs/openebs-k8s-provisioner:2.12.0
        env:
        - name: OPENEBS_ENGINE
          value: "${config.openebsEngine || 'jiva'}"
        - name: OPENEBS_REPLICA_COUNT
          value: "${config.openebsReplicaCount || 3}"
        - name: OPENEBS_FSTYPE
          value: "${config.openebsFSType || 'ext4'}"
---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ${config.storageClass}
provisioner: openebs.io/provisioner
parameters:
  openebs.io/cas-type: "${config.openebsEngine || 'jiva'}"
  openebs.io/replica-count: "${config.openebsReplicaCount || 3}"
  openebs.io/fstype: "${config.openebsFSType || 'ext4'}"
reclaimPolicy: Delete
volumeBindingMode: Immediate`;
  }

  private static generatePortworxOperatorManifest(config: StorageConfig): string {
    return `apiVersion: v1
kind: Namespace
metadata:
  name: portworx
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: portworx-operator
  namespace: portworx
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: portworx-operator
rules:
- apiGroups: ["*"]
  resources: ["*"]
  verbs: ["*"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: portworx-operator
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: portworx-operator
subjects:
- kind: ServiceAccount
  name: portworx-operator
  namespace: portworx
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: portworx-operator
  namespace: portworx
spec:
  replicas: 1
  selector:
    matchLabels:
      app: portworx-operator
  template:
    metadata:
      labels:
        app: portworx-operator
    spec:
      serviceAccountName: portworx-operator
      containers:
      - name: portworx-operator
        image: portworx/px-operator:latest
        env:
        - name: PX_CLUSTER_NAME
          value: "${config.portworxVolumeName || 'px-cluster'}"
---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ${config.storageClass}
provisioner: kubernetes.io/portworx-volume
parameters:
  repl: "${config.portworxRepl || 3}"
  fs: "${config.portworxFs || 'ext4'}"
  block_size: "${config.portworxBlockSize || '4096'}"
  queue_depth: "${config.portworxQueueDepth || 128}"
  io_profile: "${config.portworxIoProfile || 'sequential'}"
reclaimPolicy: Delete
volumeBindingMode: Immediate`;
  }
}