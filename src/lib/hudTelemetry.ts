export interface HudMetric {
  label: string;
  value: number;
  unit: string;
  trend: string;
  status: 'stable' | 'active' | 'surging';
}

export interface HudRing {
  label: string;
  value: number;
}

export interface HudNode {
  id: string;
  label: string;
  x: number;
  y: number;
  status: 'online' | 'syncing' | 'watching';
}

export interface HudToggle {
  label: string;
  enabled: boolean;
}

export interface HudTelemetry {
  metrics: HudMetric[];
  storageRings: HudRing[];
  nodes: HudNode[];
  throughputBars: number[];
  lineSeries: number[];
  toggles: HudToggle[];
  menuModes: string[];
  eventFeed: string[];
}

export function buildHudTelemetry(): HudTelemetry {
  return {
    metrics: [
      { label: 'Cluster health', value: 98, unit: '%', trend: '+4.2%', status: 'stable' },
      { label: 'Workload sync', value: 87, unit: '%', trend: '+12 ops/min', status: 'active' },
      { label: 'Manifest validity', value: 94, unit: '%', trend: 'schema clean', status: 'stable' },
      { label: 'Network mesh', value: 76, unit: '%', trend: '3 routes hot', status: 'surging' },
    ],
    storageRings: [
      { label: 'Ceph', value: 82 },
      { label: 'Longhorn', value: 68 },
      { label: 'NVMe-oF', value: 91 },
    ],
    nodes: [
      { id: 'n1', label: 'control-plane', x: 50, y: 20, status: 'online' },
      { id: 'n2', label: 'edge-a', x: 22, y: 58, status: 'syncing' },
      { id: 'n3', label: 'edge-b', x: 78, y: 58, status: 'watching' },
      { id: 'n4', label: 'vcluster', x: 50, y: 82, status: 'online' },
    ],
    throughputBars: [38, 74, 48, 89, 64, 93, 58, 81, 44, 72, 96, 69],
    lineSeries: [32, 44, 41, 68, 54, 72, 61, 88, 77, 94, 82, 97],
    toggles: [
      { label: 'Dry-run', enabled: true },
      { label: 'vCluster', enabled: true },
      { label: 'CSI', enabled: true },
      { label: 'Mesh', enabled: true },
      { label: 'Auto apply', enabled: false },
      { label: 'Audit lock', enabled: true },
    ],
    menuModes: ['Overview', 'Validate', 'Deploy', 'Observe'],
    eventFeed: [
      'control-plane accepted manifest dry-run',
      'ceph-csi provisioner heartbeat green',
      'istio mesh route telemetry streaming',
      'argocd sync target awaiting approval',
      'vcluster preview channel warming',
    ],
  };
}
