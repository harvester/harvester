import YAML from 'yaml';
import { ApplicationConfig, StorageConfig } from '../types';

export interface ManifestResource {
  apiVersion: string;
  kind: string;
  name: string;
  namespace: string;
}

export interface ValidationIssue {
  severity: 'error' | 'warning';
  message: string;
  resource?: string;
}

export interface ValidationResult {
  valid: boolean;
  resources: ManifestResource[];
  issues: ValidationIssue[];
}

export interface LivePreview {
  resourceCount: number;
  resources: ManifestResource[];
  readinessScore: number;
  summary: string;
}

export interface ApplyCheck {
  label: string;
  passed: boolean;
  detail: string;
}

export interface ApplyTestRun {
  status: 'passed' | 'failed';
  commands: string[];
  checks: ApplyCheck[];
  logs: string[];
}

export interface VClusterPlan {
  mode: 'vcluster';
  virtualClusters: { name: string; namespace: string; context: string }[];
  commands: string[];
  serviceDiscovery: string;
}

export interface CsiTemplate {
  kind: string;
  name: string;
  yaml: string;
}

export interface CsiTemplatePreview {
  driverName: string;
  storageClassName: string;
  templates: CsiTemplate[];
  installCommands: string[];
}

const SUPPORTED_KINDS = new Set([
  'Application',
  'CiliumNetworkPolicy',
  'ConfigMap',
  'CronJob',
  'DaemonSet',
  'Deployment',
  'FederatedDeployment',
  'Gateway',
  'GitRepository',
  'Ingress',
  'Job',
  'Kustomization',
  'LinkerdServiceProfile',
  'NetworkPolicy',
  'PersistentVolumeClaim',
  'PipelineActivity',
  'Role',
  'RoleBinding',
  'Secret',
  'Service',
  'ServiceAccount',
  'ServiceMonitor',
  'StatefulSet',
  'StorageClass',
]);

function parseResources(manifest: string): { resources: ManifestResource[]; issues: ValidationIssue[] } {
  const issues: ValidationIssue[] = [];
  const resources: ManifestResource[] = [];

  YAML.parseAllDocuments(manifest).forEach((document, index) => {
    if (document.errors.length > 0) {
      issues.push({
        severity: 'error',
        message: `YAML document ${index + 1} has syntax errors: ${document.errors.map((error) => error.message).join(', ')}`,
      });
      return;
    }

    const resource = document.toJSON() as Record<string, unknown> | null;
    if (!resource) {
      return;
    }

    const apiVersion = String(resource.apiVersion || '');
    const kind = String(resource.kind || '');
    const metadata = (resource.metadata || {}) as Record<string, unknown>;
    const name = String(metadata.name || '');
    const namespace = String(metadata.namespace || 'default');
    const resourceLabel = kind && name ? `${kind}/${name}` : `document ${index + 1}`;

    if (!apiVersion) {
      issues.push({ severity: 'error', message: 'Missing apiVersion', resource: resourceLabel });
    }
    if (!kind) {
      issues.push({ severity: 'error', message: 'Missing kind', resource: resourceLabel });
    }
    if (!name) {
      issues.push({ severity: 'error', message: 'Missing metadata.name', resource: resourceLabel });
    }
    if (kind && !SUPPORTED_KINDS.has(kind)) {
      issues.push({ severity: 'warning', message: `Kind ${kind} is not in the Nexus demo allow-list`, resource: resourceLabel });
    }

    if (apiVersion && kind && name) {
      resources.push({ apiVersion, kind, name, namespace });
    }
  });

  return { resources, issues };
}

export function validateKubernetesManifest(manifest: string): ValidationResult {
  const { resources, issues } = parseResources(manifest);
  const valid = issues.every((issue) => issue.severity !== 'error') && resources.length > 0;
  return { valid, resources, issues };
}

export function buildLivePreview(manifest: string): LivePreview {
  const validation = validateKubernetesManifest(manifest);
  const readinessScore = validation.valid ? Math.max(76, Math.min(99, 100 - validation.issues.length * 6)) : 35;

  return {
    resourceCount: validation.resources.length,
    resources: validation.resources,
    readinessScore,
    summary: validation.valid
      ? `${validation.resources.length} resources ready for server-side dry-run preview`
      : 'Manifest requires fixes before live preview can proceed',
  };
}

export function buildApplyTestRun(manifest: string, config: ApplicationConfig): ApplyTestRun {
  const validation = validateKubernetesManifest(manifest);
  const namespace = config.namespace || 'default';
  const commands = [
    `kubectl apply --dry-run=server -n ${namespace} -f nexus-generated.yaml`,
    `kubectl diff -n ${namespace} -f nexus-generated.yaml`,
    `kubectl apply -n ${namespace} -f nexus-generated.yaml`,
    `kubectl rollout status ${config.workloadType.toLowerCase()}/${config.appName} -n ${namespace}`,
  ];
  const checks: ApplyCheck[] = [
    {
      label: 'Schema validation',
      passed: validation.valid,
      detail: validation.valid ? 'All YAML documents include apiVersion, kind, and metadata.name' : 'Fix manifest validation errors first',
    },
    {
      label: 'Server dry-run',
      passed: validation.valid,
      detail: validation.valid ? 'Prepared for kubectl --dry-run=server' : 'Skipped until validation passes',
    },
    {
      label: 'Rollout probe',
      passed: validation.resources.some((resource) => resource.kind === config.workloadType),
      detail: `${config.workloadType}/${config.appName} rollout command generated`,
    },
  ];

  return {
    status: checks.every((check) => check.passed) ? 'passed' : 'failed',
    commands,
    checks,
    logs: checks.map((check) => `${check.passed ? 'PASS' : 'WAIT'} ${check.label}: ${check.detail}`),
  };
}

export function buildVClusterPlan(config: ApplicationConfig): VClusterPlan {
  const sourceClusters = config.multiCluster.clusters?.length
    ? config.multiCluster.clusters
    : [{ name: 'edge-a' }, { name: 'edge-b' }];
  const virtualClusters = sourceClusters.map((cluster) => ({
    name: cluster.name,
    namespace: `${cluster.name}-vcluster`,
    context: `${cluster.name}-preview`,
  }));

  return {
    mode: 'vcluster',
    virtualClusters,
    commands: virtualClusters.flatMap((cluster) => [
      `vcluster create ${cluster.name} --namespace ${cluster.namespace}`,
      `vcluster connect ${cluster.name} --namespace ${cluster.namespace} --update-current=false`,
    ]),
    serviceDiscovery: config.multiCluster.serviceDiscovery ? 'Submariner/Cilium service discovery enabled' : 'Service discovery staged for enablement',
  };
}

function driverName(storageType: StorageConfig['storageType']): string {
  const names: Record<StorageConfig['storageType'], string> = {
    local: 'rancher.io/local-path',
    nfs: 'nfs.csi.k8s.io',
    smb: 'smb.csi.k8s.io',
    ceph: 'rook-ceph.rbd.csi.ceph.com',
    nvme: 'nvme.csi.k8s.io',
    rdma: 'rdma.csi.nexus.io',
    zfs: 'zfs.csi.openebs.io',
    iscsi: 'iscsi.csi.k8s.io',
    glusterfs: 'glusterfs.csi.k8s.io',
    longhorn: 'driver.longhorn.io',
    openebs: 'openebs.io/local',
    portworx: 'pxd.portworx.com',
  };

  return names[storageType];
}

export function buildCsiTemplatePreview(storage: StorageConfig): CsiTemplatePreview {
  const provisioner = driverName(storage.storageType);
  const storageClassName = storage.storageClass || `${storage.storageType}-csi`;
  const storageClassYaml = YAML.stringify({
    apiVersion: 'storage.k8s.io/v1',
    kind: 'StorageClass',
    metadata: { name: storageClassName },
    provisioner,
    reclaimPolicy: 'Retain',
    allowVolumeExpansion: true,
    volumeBindingMode: 'WaitForFirstConsumer',
    parameters: {
      backend: storage.storageType,
      fsType: storage.openebsFSType || storage.portworxFs || 'ext4',
    },
  });
  const snapshotClassYaml = YAML.stringify({
    apiVersion: 'snapshot.storage.k8s.io/v1',
    kind: 'VolumeSnapshotClass',
    metadata: { name: `${storageClassName}-snapshots` },
    driver: provisioner,
    deletionPolicy: 'Retain',
  });
  const pvcYaml = YAML.stringify({
    apiVersion: 'v1',
    kind: 'PersistentVolumeClaim',
    metadata: { name: `${storage.storageType}-demo-pvc` },
    spec: {
      accessModes: [storage.accessMode],
      storageClassName,
      resources: { requests: { storage: storage.storageSize } },
    },
  });

  return {
    driverName: provisioner,
    storageClassName,
    templates: [
      { kind: 'StorageClass', name: storageClassName, yaml: storageClassYaml },
      { kind: 'VolumeSnapshotClass', name: `${storageClassName}-snapshots`, yaml: snapshotClassYaml },
      { kind: 'PersistentVolumeClaim', name: `${storage.storageType}-demo-pvc`, yaml: pvcYaml },
    ],
    installCommands: [
      `kubectl apply -f ${storage.storageType}-csi-driver.yaml`,
      `kubectl apply -f ${storageClassName}-storageclass.yaml`,
      `kubectl apply -f ${storage.storageType}-demo-pvc.yaml`,
    ],
  };
}
