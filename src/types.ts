export type StorageType = 'local' | 'nfs' | 'smb' | 'ceph' | 'nvme' | 'rdma' | 'zfs' | 'iscsi' | 'glusterfs' | 'longhorn' | 'openebs' | 'portworx';

export type WorkloadType = 'Deployment' | 'StatefulSet' | 'DaemonSet' | 'Job' | 'CronJob';

export type ServiceMeshType = 'istio' | 'linkerd' | 'cilium' | 'none';

export type MonitoringType = 'prometheus' | 'datadog' | 'newrelic' | 'none';

export type LoggingType = 'fluentd' | 'elasticsearch' | 'loki' | 'splunk' | 'none';

export type GitOpsType = 'argocd' | 'flux' | 'jenkinsx' | 'none';

export interface StorageConfig {
  // Common storage config
  storageType: StorageType;
  storageSize: string;
  accessMode: 'ReadWriteOnce' | 'ReadWriteMany' | 'ReadOnlyMany';
  storageClass: string;

  // NFS specific
  nfsServer?: string;
  nfsPath?: string;
  nfsVersion?: '3' | '4' | '4.1';

  // SMB specific
  smbServer?: string;
  smbShare?: string;
  smbUsername?: string;
  smbPassword?: string;
  smbDomain?: string;
  smbMultichannel?: boolean;

  // Ceph specific
  cephClusterName?: string;
  cephMonitors?: string[];
  cephPoolName?: string;
  cephFsName?: string;
  cephSecretName?: string;
  cephUser?: string;
  cephKey?: string;
  cephInstallManagers?: boolean;
  cephInstallOSDs?: boolean;
  cephInstallMDS?: boolean;
  cephInstallRGW?: boolean;
  cephManagerCount?: number;
  cephOSDCount?: number;
  cephDevices?: string[];
  cephPublicNetwork?: string;
  cephClusterNetwork?: string;

  // NVMe-oF specific
  nvmeSubsystemNQN?: string;
  nvmeTransport?: 'tcp' | 'rdma';
  nvmeTargetIP?: string;
  nvmeTargetPort?: number;
  nvmeNamespaceCount?: number;

  // RDMA specific
  rdmaDevice?: string;
  rdmaPort?: number;
  rdmaMaxQueueDepth?: number;

  // ZFS specific
  zfsPoolName?: string;
  zfsDataset?: string;
  zfsCompression?: boolean;
  zfsDedup?: boolean;
  zfsDevices?: string[];
  zfsRaidType?: 'single' | 'mirror' | 'raidz' | 'raidz2' | 'raidz3';
  zfsIscsiTarget?: string;

  // iSCSI specific
  iscsiTarget?: string;
  iscsiPortals?: string[];
  iscsiLun?: number;
  iscsiIqn?: string;
  iscsiChapUsername?: string;
  iscsiChapPassword?: string;

  // GlusterFS specific
  glusterfsServers?: string[];
  glusterfsVolume?: string;
  glusterfsTransport?: 'tcp' | 'rdma';

  // Longhorn specific
  longhornReplicaCount?: number;
  longhornDataLocality?: 'disabled' | 'best-effort' | 'strict-local';
  longhornBackingImage?: string;

  // OpenEBS specific
  openebsEngine?: 'jiva' | 'cstor' | 'localpv';
  openebsReplicaCount?: number;
  openebsFSType?: 'ext4' | 'xfs' | 'btrfs';

  // Portworx specific
  portworxVolumeName?: string;
  portworxSize?: string;
  portworxRepl?: number;
  portworxFs?: 'ext4' | 'xfs';
  portworxBlockSize?: string;
  portworxQueueDepth?: number;
  portworxIoProfile?: 'sequential' | 'random' | 'mixed';
}

export interface NetworkingConfig {
  enableService: boolean;
  serviceType: 'ClusterIP' | 'NodePort' | 'LoadBalancer';
  servicePort: number;
  enableIngress: boolean;
  hostname: string;
  ingressClass?: string;
  tlsSecret?: string;
  enableNetworkPolicy: boolean;
  networkPolicyRules?: NetworkPolicyRule[];
  serviceMesh: ServiceMeshType;
  istioInjection?: boolean;
  linkerdInjection?: boolean;
}

export interface NetworkPolicyRule {
  name: string;
  podSelector?: { [key: string]: string };
  namespaceSelector?: { [key: string]: string };
  ipBlock?: string;
  ports?: { port: number | string; protocol: 'TCP' | 'UDP' | 'SCTP' }[];
}

export interface SecurityConfig {
  enableRBAC: boolean;
  serviceAccountName?: string;
  rbacRules?: RBACRule[];
  podSecurityStandard?: 'privileged' | 'baseline' | 'restricted';
  securityContext?: {
    runAsUser?: number;
    runAsGroup?: number;
    fsGroup?: number;
    runAsNonRoot?: boolean;
  };
  networkSecurity?: {
    allowPrivilegeEscalation?: boolean;
    readOnlyRootFilesystem?: boolean;
    capabilities?: string[];
  };
}

export interface RBACRule {
  apiGroups: string[];
  resources: string[];
  verbs: string[];
}

export interface MonitoringConfig {
  monitoring: MonitoringType;
  enableMetrics: boolean;
  metricsPath?: string;
  prometheusAnnotations?: boolean;
  customMetrics?: string[];
  enableTracing?: boolean;
  tracingEndpoint?: string;
}

export interface LoggingConfig {
  logging: LoggingType;
  logLevel?: 'debug' | 'info' | 'warn' | 'error';
  logFormat?: 'json' | 'logfmt' | 'text';
  enableStructuredLogging?: boolean;
  logRetention?: string;
}

export interface GitOpsConfig {
  gitOps: GitOpsType;
  gitRepository?: string;
  repoUrl?: string;
  path?: string;
  gitBranch?: string;
  syncInterval?: number;
  autoSync?: boolean;
  prune?: boolean;
}

export interface MultiClusterConfig {
  enableMultiCluster: boolean;
  clusters?: ClusterConfig[];
  federationType?: 'kubefed' | 'submariner' | 'cilium';
  serviceDiscovery?: boolean;
  loadBalancing?: 'round-robin' | 'least-connections' | 'ip-hash';
}

export interface ClusterConfig {
  name: string;
  kubeconfig?: string;
  context?: string;
  region?: string;
  zone?: string;
  labels?: { [key: string]: string };
}

export interface ApplicationConfig {
  appName: string;
  namespace: string;
  workloadType: WorkloadType;
  image: string;
  replicas: number;
  cpuRequest: string;
  memoryRequest: string;
  cpuLimit: string;
  memoryLimit: string;
  storage: StorageConfig;
  networking: NetworkingConfig;
  security: SecurityConfig;
  monitoring: MonitoringConfig;
  logging: LoggingConfig;
  gitOps: GitOpsConfig;
  multiCluster: MultiClusterConfig;
  enableHealthChecks: boolean;
  healthCheckPath?: string;
  readinessProbe?: ProbeConfig;
  livenessProbe?: ProbeConfig;
  startupProbe?: ProbeConfig;
  environmentVariables?: { [key: string]: string };
  configMaps?: string[];
  secrets?: string[];
  annotations?: { [key: string]: string };
  labels?: { [key: string]: string };
}

export interface ProbeConfig {
  httpGet?: {
    path: string;
    port: number;
    scheme?: 'HTTP' | 'HTTPS';
  };
  tcpSocket?: {
    port: number;
  };
  exec?: {
    command: string[];
  };
  initialDelaySeconds?: number;
  periodSeconds?: number;
  timeoutSeconds?: number;
  successThreshold?: number;
  failureThreshold?: number;
}

export const defaultConfig: ApplicationConfig = {
  appName: 'example-app',
  namespace: 'default',
  workloadType: 'Deployment',
  image: 'nginx:stable',
  replicas: 2,
  cpuRequest: '100m',
  memoryRequest: '128Mi',
  cpuLimit: '400m',
  memoryLimit: '512Mi',
  storage: {
    storageType: 'local',
    storageSize: '5Gi',
    accessMode: 'ReadWriteOnce',
    storageClass: 'standard',
  },
  networking: {
    enableService: true,
    serviceType: 'ClusterIP',
    servicePort: 80,
    enableIngress: false,
    hostname: 'example.local',
    enableNetworkPolicy: false,
    serviceMesh: 'none',
  },
  security: {
    enableRBAC: false,
    podSecurityStandard: 'baseline',
  },
  monitoring: {
    monitoring: 'none',
    enableMetrics: false,
  },
  logging: {
    logging: 'none',
  },
  gitOps: {
    gitOps: 'none',
  },
  multiCluster: {
    enableMultiCluster: false,
  },
  enableHealthChecks: true,
  healthCheckPath: '/health',
  environmentVariables: {},
  configMaps: [],
  secrets: [],
  annotations: {},
  labels: {},
};
