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

export interface HudStatusRail {
  label: string;
  value: number;
}

export interface HudRadioGroup {
  label: string;
  options: { label: string; active: boolean }[];
}

export interface HudScanPanel {
  label: string;
  value: string;
  bars: number[];
}

export interface HudTelemetry {
  metrics: HudMetric[];
  storageRings: HudRing[];
  nodes: HudNode[];
  throughputBars: number[];
  lineSeries: number[];
  toggles: HudToggle[];
  menuModes: string[];
  statusRails: HudStatusRail[];
  radioGroups: HudRadioGroup[];
  scanPanels: HudScanPanel[];
  microLabels: string[];
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
    statusRails: [
      { label: 'RAD_CP', value: 82 },
      { label: 'API_TX', value: 74 },
      { label: 'CSI_IO', value: 91 },
      { label: 'MESH_RT', value: 68 },
      { label: 'VC_SYNC', value: 88 },
      { label: 'AUDIT', value: 79 },
    ],
    radioGroups: [
      {
        label: 'Mode matrix',
        options: [
          { label: 'A1', active: true },
          { label: 'A2', active: false },
          { label: 'A3', active: true },
          { label: 'A4', active: false },
        ],
      },
      {
        label: 'Routing',
        options: [
          { label: 'R1', active: false },
          { label: 'R2', active: true },
          { label: 'R3', active: true },
          { label: 'R4', active: false },
        ],
      },
      {
        label: 'Apply gate',
        options: [
          { label: 'G1', active: true },
          { label: 'G2', active: true },
          { label: 'G3', active: false },
          { label: 'G4', active: true },
        ],
      },
    ],
    scanPanels: [
      { label: 'Volume scan', value: '92_002', bars: [62, 47, 80, 70, 88] },
      { label: 'Mesh scan', value: 'TS_23.45', bars: [35, 76, 52, 91, 64] },
    ],
    microLabels: ['X_300.1', 'RAD_CP_02.2', 'AUTOMATION SECT 04', 'MEMBRANE_SECT10', 'TS_23.45'],
    eventFeed: [
      'control-plane accepted manifest dry-run',
      'ceph-csi provisioner heartbeat green',
      'istio mesh route telemetry streaming',
      'argocd sync target awaiting approval',
      'vcluster preview channel warming',
    ],
  };
}
