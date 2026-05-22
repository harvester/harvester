import { describe, expect, it } from 'vitest';
import { defaultConfig } from '../types';
import {
  buildApplyTestRun,
  buildCsiTemplatePreview,
  buildLivePreview,
  buildVClusterPlan,
  validateKubernetesManifest,
} from './clusterWorkflow';

const manifest = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo-app
  namespace: default
spec: {}
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: demo-app-pvc
spec: {}
`;

describe('cluster workflow helpers', () => {
  it('validates Kubernetes manifests and returns a live resource preview', () => {
    const validation = validateKubernetesManifest(manifest);
    const preview = buildLivePreview(manifest);

    expect(validation.valid).toBe(true);
    expect(validation.resources).toHaveLength(2);
    expect(preview.resourceCount).toBe(2);
    expect(preview.resources.map((resource) => resource.kind)).toEqual(['Deployment', 'PersistentVolumeClaim']);
  });

  it('builds a kubectl dry-run/apply test run with commands and successful checks', () => {
    const run = buildApplyTestRun(manifest, defaultConfig);

    expect(run.status).toBe('passed');
    expect(run.commands[0]).toContain('kubectl apply --dry-run=server');
    expect(run.checks.every((check) => check.passed)).toBe(true);
  });

  it('builds vcluster and CSI driver previews from application configuration', () => {
    const config = {
      ...defaultConfig,
      storage: { ...defaultConfig.storage, storageType: 'ceph' as const },
      multiCluster: {
        enableMultiCluster: true,
        clusters: [{ name: 'edge-a' }, { name: 'edge-b' }],
        federationType: 'kubefed' as const,
        serviceDiscovery: true,
      },
    };

    const vclusterPlan = buildVClusterPlan(config);
    const csiPreview = buildCsiTemplatePreview(config.storage);

    expect(vclusterPlan.virtualClusters.map((cluster) => cluster.name)).toEqual(['edge-a', 'edge-b']);
    expect(vclusterPlan.commands[0]).toContain('vcluster create edge-a');
    expect(csiPreview.driverName).toContain('rook-ceph');
    expect(csiPreview.templates.some((template) => template.kind === 'StorageClass')).toBe(true);
  });
});
