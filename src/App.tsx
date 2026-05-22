import { useMemo, useState } from 'react';
import { ApplicationConfig, defaultConfig, StorageType } from './types';
import { generateManifest } from './lib/manifestGenerator';
import { buildApplyTestRun, buildCsiTemplatePreview, buildLivePreview, buildVClusterPlan, validateKubernetesManifest } from './lib/clusterWorkflow';
import { isDemoLogin } from './lib/auth';
import { ClusterIntegrationPanel } from './components/ClusterIntegrationPanel';
import { LoginScreen } from './components/LoginScreen';
import { HudDashboard } from './components/HudDashboard';
import { Wizard } from './components/Wizard';
import { YamlEditor } from './components/YamlEditor';

const STORAGE_TEMPLATES: Record<StorageType, string> = {
  local: 'Local path provisioning with hostPath / local-path-provisioner',
  nfs: 'NFS client provisioner with PersistentVolumeClaim',
  smb: 'SMB CSI driver for CIFS shares',
  ceph: 'Rook Ceph block/storage classes for CephFS or RBD',
  nvme: 'NVMe-oF over TCP volume claim',
  rdma: 'RDMA-backed CSI volume',
  zfs: 'ZFS over iSCSI or ZFS CSI driver',
  iscsi: 'iSCSI block storage with CSI driver',
  glusterfs: 'GlusterFS distributed filesystem with CSI',
  longhorn: 'Longhorn cloud-native distributed block storage',
  openebs: 'OpenEBS container-native storage with multiple engines',
  portworx: 'Portworx enterprise container storage platform',
};

function App() {
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [config, setConfig] = useState<ApplicationConfig>(defaultConfig);
  const [step, setStep] = useState(1);
  const [editedYaml, setEditedYaml] = useState('');

  const manifest = useMemo(() => generateManifest(config), [config]);
  const displayedManifest = editedYaml || manifest;
  const validation = useMemo(() => validateKubernetesManifest(displayedManifest), [displayedManifest]);
  const livePreview = useMemo(() => buildLivePreview(displayedManifest), [displayedManifest]);
  const applyRun = useMemo(() => buildApplyTestRun(displayedManifest, config), [displayedManifest, config]);
  const vclusterPlan = useMemo(() => buildVClusterPlan(config), [config]);
  const csiPreview = useMemo(() => buildCsiTemplatePreview(config.storage), [config.storage]);

  if (!isAuthenticated) {
    return (
      <LoginScreen
        onLogin={(username, password) => {
          const loginAccepted = isDemoLogin(username, password);
          setIsAuthenticated(loginAccepted);
          return loginAccepted;
        }}
      />
    );
  }

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <div className="brand-icon">NX</div>
          <div>
            <h1>Nexus</h1>
            <p>Dark-mode workload & storage manifest generator</p>
          </div>
        </div>
        <div className="step-list">
          <button className={step === 1 ? 'active' : ''} onClick={() => setStep(1)}>
            1. Workload
          </button>
          <button className={step === 2 ? 'active' : ''} onClick={() => setStep(2)}>
            2. Storage
          </button>
          <button className={step === 3 ? 'active' : ''} onClick={() => setStep(3)}>
            3. Networking
          </button>
          <button className={step === 4 ? 'active' : ''} onClick={() => setStep(4)}>
            4. Security
          </button>
          <button className={step === 5 ? 'active' : ''} onClick={() => setStep(5)}>
            5. Monitoring
          </button>
          <button className={step === 6 ? 'active' : ''} onClick={() => setStep(6)}>
            6. GitOps
          </button>
          <button className={step === 7 ? 'active' : ''} onClick={() => setStep(7)}>
            7. Review
          </button>
        </div>
        <div className="storage-summary">
          <h2>Storage template</h2>
          <p>{STORAGE_TEMPLATES[config.storage.storageType]}</p>
        </div>
      </aside>
      <main className="main-view">
        <HudDashboard />
        <ClusterIntegrationPanel
          validation={validation}
          livePreview={livePreview}
          applyRun={applyRun}
          vclusterPlan={vclusterPlan}
          csiPreview={csiPreview}
          config={config}
        />
        <Wizard currentStep={step} config={config} onChange={setConfig} onNext={() => setStep(Math.min(step + 1, 7))} onBack={() => setStep(Math.max(step - 1, 1))} />
        <section className="manifest-panel">
          <div className="panel-header">
            <h2>Generated manifest</h2>
            <span className="badge">Kubernetes 1.28+</span>
          </div>
          <YamlEditor value={displayedManifest} onChange={setEditedYaml} validationIssues={validation.issues.map((issue) => issue.message)} />
        </section>
      </main>
    </div>
  );
}

export default App;
