import { ApplicationConfig, StorageType, WorkloadType } from '../types';

interface WizardProps {
  currentStep: number;
  config: ApplicationConfig;
  onChange: (config: ApplicationConfig) => void;
  onNext: () => void;
  onBack: () => void;
}

const storageOptions: { value: StorageType; label: string; description: string }[] = [
  { value: 'local', label: 'Local', description: 'Local path storage for single-node or dev environments' },
  { value: 'nfs', label: 'NFS', description: 'Shared file storage using NFS' },
  { value: 'smb', label: 'SMB', description: 'SMB/CIFS file share with multichannel-ready CSI support' },
  { value: 'ceph', label: 'Ceph', description: 'Distributed Ceph/Rook backend for block and filesystem' },
  { value: 'nvme', label: 'NVMe-oF', description: 'NVMe over TCP with high-performance block storage' },
  { value: 'rdma', label: 'RDMA', description: 'Low-latency RDMA-backed CSI volumes' },
  { value: 'zfs', label: 'ZFS', description: 'ZFS-backed persistent volumes via CSI or iSCSI' },
  { value: 'iscsi', label: 'iSCSI', description: 'Block storage via iSCSI protocol' },
  { value: 'glusterfs', label: 'GlusterFS', description: 'Distributed filesystem with CSI support' },
  { value: 'longhorn', label: 'Longhorn', description: 'Cloud-native distributed block storage' },
  { value: 'openebs', label: 'OpenEBS', description: 'Container-native storage with multiple engines' },
  { value: 'portworx', label: 'Portworx', description: 'Enterprise-grade container storage platform' },
];

const workloadOptions: WorkloadType[] = ['Deployment', 'StatefulSet', 'DaemonSet', 'Job', 'CronJob'];

export function Wizard({ currentStep, config, onChange, onNext, onBack }: WizardProps) {
  return (
    <section className="wizard-shell">
      <header className="wizard-header">
        <h2>Step {currentStep} of 7</h2>
        <p>
          {currentStep === 1 && 'Configure your application basics and workload type.'}
          {currentStep === 2 && 'Select and configure storage backend for your application.'}
          {currentStep === 3 && 'Configure networking, services, and ingress.'}
          {currentStep === 4 && 'Set up security policies and RBAC.'}
          {currentStep === 5 && 'Configure monitoring and logging solutions.'}
          {currentStep === 6 && 'Set up GitOps and multi-cluster deployment.'}
          {currentStep === 7 && 'Review your configuration before generating manifests.'}
        </p>
      </header>
      {currentStep === 1 && (
        <div className="wizard-panel">
          <label>
            Application name
            <input value={config.appName} onChange={(e) => onChange({ ...config, appName: e.target.value })} />
          </label>
          <label>
            Namespace
            <input value={config.namespace} onChange={(e) => onChange({ ...config, namespace: e.target.value })} />
          </label>
          <label>
            Workload type
            <select value={config.workloadType} onChange={(e) => onChange({ ...config, workloadType: e.target.value as WorkloadType })}>
              {workloadOptions.map((type) => (
                <option key={type} value={type}>{type}</option>
              ))}
            </select>
          </label>
          <label>
            Container image
            <input value={config.image} onChange={(e) => onChange({ ...config, image: e.target.value })} />
          </label>
          <div className="grid-2">
            <label>
              Replicas
              <input type="number" min={1} value={config.replicas} onChange={(e) => onChange({ ...config, replicas: Number(e.target.value) })} />
            </label>
            <label>
              Service port
              <input type="number" min={1} value={config.networking.servicePort} onChange={(e) => onChange({ ...config, networking: { ...config.networking, servicePort: Number(e.target.value) } })} />
            </label>
          </div>
          <div className="grid-4">
            <label>
              CPU request
              <input value={config.cpuRequest} onChange={(e) => onChange({ ...config, cpuRequest: e.target.value })} />
            </label>
            <label>
              Memory request
              <input value={config.memoryRequest} onChange={(e) => onChange({ ...config, memoryRequest: e.target.value })} />
            </label>
            <label>
              CPU limit
              <input value={config.cpuLimit} onChange={(e) => onChange({ ...config, cpuLimit: e.target.value })} />
            </label>
            <label>
              Memory limit
              <input value={config.memoryLimit} onChange={(e) => onChange({ ...config, memoryLimit: e.target.value })} />
            </label>
          </div>
        </div>
      )}

      {currentStep === 2 && (
        <div className="wizard-panel">
          <h3>Storage selection</h3>
          <div className="storage-grid">
            {storageOptions.map((option) => (
              <button
                key={option.value}
                className={option.value === config.storage.storageType ? 'storage-option active' : 'storage-option'}
                onClick={() => onChange({ ...config, storage: { ...config.storage, storageType: option.value } })}
                type="button"
              >
                <strong>{option.label}</strong>
                <span>{option.description}</span>
              </button>
            ))}
          </div>
          <label>
            Storage class
            <input value={config.storage.storageClass} onChange={(e) => onChange({ ...config, storage: { ...config.storage, storageClass: e.target.value } })} />
          </label>
          <div className="grid-2">
            <label>
              Size
              <input value={config.storage.storageSize} onChange={(e) => onChange({ ...config, storage: { ...config.storage, storageSize: e.target.value } })} />
            </label>
            <label>
              Access mode
              <select value={config.storage.accessMode} onChange={(e) => onChange({ ...config, storage: { ...config.storage, accessMode: e.target.value as ApplicationConfig['storage']['accessMode'] } })}>
                <option value="ReadWriteOnce">ReadWriteOnce</option>
                <option value="ReadWriteMany">ReadWriteMany</option>
                <option value="ReadOnlyMany">ReadOnlyMany</option>
              </select>
            </label>
          </div>

          {/* Storage-specific configuration */}
          {config.storage.storageType === 'nfs' && (
            <div className="storage-config-section">
              <h4>NFS Configuration</h4>
              <div className="grid-2">
                <label>
                  NFS Server
                  <input
                    value={config.storage.nfsServer || ''}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, nfsServer: e.target.value } })}
                    placeholder="192.168.1.100"
                  />
                </label>
                <label>
                  NFS Path
                  <input
                    value={config.storage.nfsPath || ''}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, nfsPath: e.target.value } })}
                    placeholder="/exports/data"
                  />
                </label>
              </div>
              <label>
                NFS Version
                <select
                  value={config.storage.nfsVersion || '4.1'}
                  onChange={(e) => onChange({ ...config, storage: { ...config.storage, nfsVersion: e.target.value as '3' | '4' | '4.1' } })}
                >
                  <option value="3">NFSv3</option>
                  <option value="4">NFSv4</option>
                  <option value="4.1">NFSv4.1</option>
                </select>
              </label>
            </div>
          )}

          {config.storage.storageType === 'smb' && (
            <div className="storage-config-section">
              <h4>SMB Configuration</h4>
              <div className="grid-2">
                <label>
                  SMB Server
                  <input
                    value={config.storage.smbServer || ''}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, smbServer: e.target.value } })}
                    placeholder="fileserver.company.com"
                  />
                </label>
                <label>
                  SMB Share
                  <input
                    value={config.storage.smbShare || ''}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, smbShare: e.target.value } })}
                    placeholder="shared"
                  />
                </label>
              </div>
              <div className="grid-3">
                <label>
                  Username
                  <input
                    value={config.storage.smbUsername || ''}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, smbUsername: e.target.value } })}
                    placeholder="smbuser"
                  />
                </label>
                <label>
                  Password
                  <input
                    type="password"
                    value={config.storage.smbPassword || ''}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, smbPassword: e.target.value } })}
                    placeholder="password"
                  />
                </label>
                <label>
                  Domain
                  <input
                    value={config.storage.smbDomain || ''}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, smbDomain: e.target.value } })}
                    placeholder="WORKGROUP"
                  />
                </label>
              </div>
              <label className="checkbox-label">
                <input
                  type="checkbox"
                  checked={config.storage.smbMultichannel || false}
                  onChange={(e) => onChange({ ...config, storage: { ...config.storage, smbMultichannel: e.target.checked } })}
                />
                Enable SMB Multichannel
              </label>
            </div>
          )}

          {config.storage.storageType === 'ceph' && (
            <div className="storage-config-section">
              <h4>Ceph Configuration</h4>
              <div className="grid-2">
                <label>
                  Cluster Name
                  <input
                    value={config.storage.cephClusterName || ''}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, cephClusterName: e.target.value } })}
                    placeholder="rook-ceph"
                  />
                </label>
                <label>
                  Pool Name
                  <input
                    value={config.storage.cephPoolName || ''}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, cephPoolName: e.target.value } })}
                    placeholder="kubernetes"
                  />
                </label>
              </div>
              <div className="grid-3">
                <label>
                  Manager Count
                  <input
                    type="number"
                    min={1}
                    max={5}
                    value={config.storage.cephManagerCount || 2}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, cephManagerCount: Number(e.target.value) } })}
                  />
                </label>
                <label>
                  OSD Count
                  <input
                    type="number"
                    min={1}
                    max={10}
                    value={config.storage.cephOSDCount || 3}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, cephOSDCount: Number(e.target.value) } })}
                  />
                </label>
                <label>
                  FS Name
                  <input
                    value={config.storage.cephFsName || ''}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, cephFsName: e.target.value } })}
                    placeholder="myfs"
                  />
                </label>
              </div>
              <div className="ceph-components">
                <label className="checkbox-label">
                  <input
                    type="checkbox"
                    checked={config.storage.cephInstallManagers || false}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, cephInstallManagers: e.target.checked } })}
                  />
                  Install Ceph Managers
                </label>
                <label className="checkbox-label">
                  <input
                    type="checkbox"
                    checked={config.storage.cephInstallOSDs || false}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, cephInstallOSDs: e.target.checked } })}
                  />
                  Install OSDs
                </label>
                <label className="checkbox-label">
                  <input
                    type="checkbox"
                    checked={config.storage.cephInstallMDS || false}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, cephInstallMDS: e.target.checked } })}
                  />
                  Install MDS (CephFS)
                </label>
                <label className="checkbox-label">
                  <input
                    type="checkbox"
                    checked={config.storage.cephInstallRGW || false}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, cephInstallRGW: e.target.checked } })}
                  />
                  Install RGW (Object Gateway)
                </label>
              </div>
            </div>
          )}

          {config.storage.storageType === 'nvme' && (
            <div className="storage-config-section">
              <h4>NVMe-oF Configuration</h4>
              <div className="grid-3">
                <label>
                  Subsystem NQN
                  <input
                    value={config.storage.nvmeSubsystemNQN || ''}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, nvmeSubsystemNQN: e.target.value } })}
                    placeholder="nqn.2014-08.org.nvmexpress:uuid:12345678-1234-1234-1234-123456789012"
                  />
                </label>
                <label>
                  Transport
                  <select
                    value={config.storage.nvmeTransport || 'tcp'}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, nvmeTransport: e.target.value as 'tcp' | 'rdma' } })}
                  >
                    <option value="tcp">TCP</option>
                    <option value="rdma">RDMA</option>
                  </select>
                </label>
                <label>
                  Target Port
                  <input
                    type="number"
                    value={config.storage.nvmeTargetPort || 4420}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, nvmeTargetPort: Number(e.target.value) } })}
                  />
                </label>
              </div>
              <label>
                Target IP
                <input
                  value={config.storage.nvmeTargetIP || ''}
                  onChange={(e) => onChange({ ...config, storage: { ...config.storage, nvmeTargetIP: e.target.value } })}
                  placeholder="192.168.1.100"
                />
              </label>
            </div>
          )}

          {config.storage.storageType === 'zfs' && (
            <div className="storage-config-section">
              <h4>ZFS Configuration</h4>
              <div className="grid-2">
                <label>
                  Pool Name
                  <input
                    value={config.storage.zfsPoolName || ''}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, zfsPoolName: e.target.value } })}
                    placeholder="tank"
                  />
                </label>
                <label>
                  Dataset
                  <input
                    value={config.storage.zfsDataset || ''}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, zfsDataset: e.target.value } })}
                    placeholder="kubernetes"
                  />
                </label>
              </div>
              <div className="grid-3">
                <label>
                  RAID Type
                  <select
                    value={config.storage.zfsRaidType || 'single'}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, zfsRaidType: e.target.value as 'single' | 'mirror' | 'raidz' | 'raidz2' | 'raidz3' } })}
                  >
                    <option value="single">Single</option>
                    <option value="mirror">Mirror</option>
                    <option value="raidz">RAID-Z</option>
                    <option value="raidz2">RAID-Z2</option>
                    <option value="raidz3">RAID-Z3</option>
                  </select>
                </label>
                <label className="checkbox-label">
                  <input
                    type="checkbox"
                    checked={config.storage.zfsCompression || false}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, zfsCompression: e.target.checked } })}
                  />
                  Enable Compression
                </label>
                <label className="checkbox-label">
                  <input
                    type="checkbox"
                    checked={config.storage.zfsDedup || false}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, zfsDedup: e.target.checked } })}
                  />
                  Enable Deduplication
                </label>
              </div>
              <label>
                Device Paths (comma-separated)
                <input
                  value={config.storage.zfsDevices?.join(', ') || ''}
                  onChange={(e) => onChange({ ...config, storage: { ...config.storage, zfsDevices: e.target.value.split(',').map(d => d.trim()) } })}
                  placeholder="/dev/sda, /dev/sdb, /dev/sdc"
                />
              </label>
            </div>
          )}

          {config.storage.storageType === 'iscsi' && (
            <div className="storage-config-section">
              <h4>iSCSI Configuration</h4>
              <div className="grid-3">
                <label>
                  Target Portal
                  <input
                    value={config.storage.iscsiTarget || ''}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, iscsiTarget: e.target.value } })}
                    placeholder="192.168.1.100:3260"
                  />
                </label>
                <label>
                  IQN
                  <input
                    value={config.storage.iscsiIqn || ''}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, iscsiIqn: e.target.value } })}
                    placeholder="iqn.2020-01.com.example:target"
                  />
                </label>
                <label>
                  LUN
                  <input
                    type="number"
                    value={config.storage.iscsiLun || 0}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, iscsiLun: Number(e.target.value) } })}
                  />
                </label>
              </div>
            </div>
          )}

          {config.storage.storageType === 'glusterfs' && (
            <div className="storage-config-section">
              <h4>GlusterFS Configuration</h4>
              <div className="grid-2">
                <label>
                  GlusterFS Servers
                  <input
                    value={config.storage.glusterfsServers?.join(', ') || ''}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, glusterfsServers: e.target.value.split(',').map(s => s.trim()) } })}
                    placeholder="192.168.1.100, 192.168.1.101"
                  />
                </label>
                <label>
                  Volume Name
                  <input
                    value={config.storage.glusterfsVolume || ''}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, glusterfsVolume: e.target.value } })}
                    placeholder="vol1"
                  />
                </label>
              </div>
              <label>
                Transport
                <select
                  value={config.storage.glusterfsTransport || 'tcp'}
                  onChange={(e) => onChange({ ...config, storage: { ...config.storage, glusterfsTransport: e.target.value as 'tcp' | 'rdma' } })}
                >
                  <option value="tcp">TCP</option>
                  <option value="rdma">RDMA</option>
                </select>
              </label>
            </div>
          )}

          {config.storage.storageType === 'longhorn' && (
            <div className="storage-config-section">
              <h4>Longhorn Configuration</h4>
              <div className="grid-3">
                <label>
                  Replica Count
                  <input
                    type="number"
                    min={1}
                    max={10}
                    value={config.storage.longhornReplicaCount || 3}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, longhornReplicaCount: Number(e.target.value) } })}
                  />
                </label>
                <label>
                  Data Locality
                  <select
                    value={config.storage.longhornDataLocality || 'disabled'}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, longhornDataLocality: e.target.value as 'disabled' | 'best-effort' | 'strict-local' } })}
                  >
                    <option value="disabled">Disabled</option>
                    <option value="best-effort">Best Effort</option>
                    <option value="strict-local">Strict Local</option>
                  </select>
                </label>
                <label>
                  Backing Image
                  <input
                    value={config.storage.longhornBackingImage || ''}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, longhornBackingImage: e.target.value } })}
                    placeholder="ubuntu-20.04.img"
                  />
                </label>
              </div>
            </div>
          )}

          {config.storage.storageType === 'openebs' && (
            <div className="storage-config-section">
              <h4>OpenEBS Configuration</h4>
              <div className="grid-3">
                <label>
                  Engine
                  <select
                    value={config.storage.openebsEngine || 'jiva'}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, openebsEngine: e.target.value as 'jiva' | 'cstor' | 'localpv' } })}
                  >
                    <option value="jiva">Jiva</option>
                    <option value="cstor">cStor</option>
                    <option value="localpv">Local PV</option>
                  </select>
                </label>
                <label>
                  Replica Count
                  <input
                    type="number"
                    min={1}
                    max={5}
                    value={config.storage.openebsReplicaCount || 3}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, openebsReplicaCount: Number(e.target.value) } })}
                  />
                </label>
                <label>
                  Filesystem Type
                  <select
                    value={config.storage.openebsFSType || 'ext4'}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, openebsFSType: e.target.value as 'ext4' | 'xfs' | 'btrfs' } })}
                  >
                    <option value="ext4">ext4</option>
                    <option value="xfs">XFS</option>
                    <option value="btrfs">Btrfs</option>
                  </select>
                </label>
              </div>
            </div>
          )}

          {config.storage.storageType === 'portworx' && (
            <div className="storage-config-section">
              <h4>Portworx Configuration</h4>
              <div className="grid-4">
                <label>
                  Replication Factor
                  <input
                    type="number"
                    min={1}
                    max={3}
                    value={config.storage.portworxRepl || 3}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, portworxRepl: Number(e.target.value) } })}
                  />
                </label>
                <label>
                  Filesystem Type
                  <select
                    value={config.storage.portworxFs || 'ext4'}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, portworxFs: e.target.value as 'ext4' | 'xfs' } })}
                  >
                    <option value="ext4">ext4</option>
                    <option value="xfs">XFS</option>
                  </select>
                </label>
                <label>
                  Block Size
                  <input
                    value={config.storage.portworxBlockSize || '4096'}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, portworxBlockSize: e.target.value } })}
                    placeholder="4096"
                  />
                </label>
                <label>
                  Queue Depth
                  <input
                    type="number"
                    value={config.storage.portworxQueueDepth || 128}
                    onChange={(e) => onChange({ ...config, storage: { ...config.storage, portworxQueueDepth: Number(e.target.value) } })}
                  />
                </label>
              </div>
              <label>
                I/O Profile
                <select
                  value={config.storage.portworxIoProfile || 'sequential'}
                  onChange={(e) => onChange({ ...config, storage: { ...config.storage, portworxIoProfile: e.target.value as 'sequential' | 'random' | 'mixed' } })}
                >
                  <option value="sequential">Sequential</option>
                  <option value="random">Random</option>
                  <option value="mixed">Mixed</option>
                </select>
              </label>
            </div>
          )}

          <label className="checkbox-label">
            <input type="checkbox" checked={config.networking.enableIngress} onChange={(e) => onChange({ ...config, networking: { ...config.networking, enableIngress: e.target.checked } })} />
            Generate Ingress object
          </label>
          {config.networking.enableIngress && (
            <label>
              Hostname
              <input value={config.networking.hostname} onChange={(e) => onChange({ ...config, networking: { ...config.networking, hostname: e.target.value } })} />
            </label>
          )}
        </div>
      )}

      {currentStep === 3 && (
        <div className="wizard-panel">
          <h3>Networking Configuration</h3>
          <div className="grid-2">
            <label className="checkbox-label">
              <input type="checkbox" checked={config.networking.enableService} onChange={(e) => onChange({ ...config, networking: { ...config.networking, enableService: e.target.checked } })} />
              Enable Service
            </label>
            <label className="checkbox-label">
              <input type="checkbox" checked={config.networking.enableNetworkPolicy} onChange={(e) => onChange({ ...config, networking: { ...config.networking, enableNetworkPolicy: e.target.checked } })} />
              Enable Network Policy
            </label>
          </div>
          {config.networking.enableService && (
            <div className="grid-2">
              <label>
                Service Type
                <select value={config.networking.serviceType} onChange={(e) => onChange({ ...config, networking: { ...config.networking, serviceType: e.target.value as 'ClusterIP' | 'NodePort' | 'LoadBalancer' } })}>
                  <option value="ClusterIP">ClusterIP</option>
                  <option value="NodePort">NodePort</option>
                  <option value="LoadBalancer">LoadBalancer</option>
                </select>
              </label>
              <label>
                Service Port
                <input type="number" min={1} value={config.networking.servicePort} onChange={(e) => onChange({ ...config, networking: { ...config.networking, servicePort: Number(e.target.value) } })} />
              </label>
            </div>
          )}
          <label>
            Service Mesh
            <select value={config.networking.serviceMesh} onChange={(e) => onChange({ ...config, networking: { ...config.networking, serviceMesh: e.target.value as 'none' | 'istio' | 'linkerd' | 'cilium' } })}>
              <option value="none">None</option>
              <option value="istio">Istio</option>
              <option value="linkerd">Linkerd</option>
              <option value="cilium">Cilium</option>
            </select>
          </label>
        </div>
      )}

      {currentStep === 4 && (
        <div className="wizard-panel">
          <h3>Security Configuration</h3>
          <label className="checkbox-label">
            <input type="checkbox" checked={config.security.enableRBAC} onChange={(e) => onChange({ ...config, security: { ...config.security, enableRBAC: e.target.checked } })} />
            Enable RBAC (Role-Based Access Control)
          </label>
          <label>
            Pod Security Standard
            <select value={config.security.podSecurityStandard} onChange={(e) => onChange({ ...config, security: { ...config.security, podSecurityStandard: e.target.value as 'privileged' | 'baseline' | 'restricted' } })}>
              <option value="privileged">Privileged</option>
              <option value="baseline">Baseline</option>
              <option value="restricted">Restricted</option>
            </select>
          </label>
        </div>
      )}

      {currentStep === 5 && (
        <div className="wizard-panel">
          <h3>Monitoring & Logging</h3>
          <div className="grid-2">
            <label>
              Monitoring
              <select value={config.monitoring.monitoring} onChange={(e) => onChange({ ...config, monitoring: { ...config.monitoring, monitoring: e.target.value as 'none' | 'prometheus' | 'datadog' | 'newrelic' } })}>
                <option value="none">None</option>
                <option value="prometheus">Prometheus</option>
                <option value="datadog">Datadog</option>
                <option value="newrelic">New Relic</option>
              </select>
            </label>
            <label>
              Logging
              <select value={config.logging.logging} onChange={(e) => onChange({ ...config, logging: { ...config.logging, logging: e.target.value as 'none' | 'fluentd' | 'loki' | 'splunk' } })}>
                <option value="none">None</option>
                <option value="fluentd">Fluentd</option>
                <option value="loki">Loki</option>
                <option value="splunk">Splunk</option>
              </select>
            </label>
          </div>
          <label className="checkbox-label">
            <input type="checkbox" checked={config.monitoring.enableMetrics} onChange={(e) => onChange({ ...config, monitoring: { ...config.monitoring, enableMetrics: e.target.checked } })} />
            Enable Metrics Collection
          </label>
          {config.monitoring.enableMetrics && (
            <label>
              Metrics Path
              <input value={config.monitoring.metricsPath || '/metrics'} onChange={(e) => onChange({ ...config, monitoring: { ...config.monitoring, metricsPath: e.target.value } })} />
            </label>
          )}
        </div>
      )}

      {currentStep === 6 && (
        <div className="wizard-panel">
          <h3>GitOps & Multi-Cluster</h3>
          <div className="grid-2">
            <label>
              GitOps
              <select value={config.gitOps.gitOps} onChange={(e) => onChange({ ...config, gitOps: { ...config.gitOps, gitOps: e.target.value as 'none' | 'argocd' | 'flux' | 'jenkinsx' } })}>
                <option value="none">None</option>
                <option value="argocd">ArgoCD</option>
                <option value="flux">Flux</option>
                <option value="jenkinsx">Jenkins X</option>
              </select>
            </label>
            <label className="checkbox-label">
              <input type="checkbox" checked={config.multiCluster.enableMultiCluster} onChange={(e) => onChange({ ...config, multiCluster: { ...config.multiCluster, enableMultiCluster: e.target.checked } })} />
              Enable Multi-Cluster
            </label>
          </div>
          {config.gitOps.gitOps !== 'none' && (
            <div className="grid-2">
              <label>
                Repository URL
                <input value={config.gitOps.repoUrl || ''} onChange={(e) => onChange({ ...config, gitOps: { ...config.gitOps, repoUrl: e.target.value } })} placeholder="https://github.com/user/repo" />
              </label>
              <label>
                Path
                <input value={config.gitOps.path || '.'} onChange={(e) => onChange({ ...config, gitOps: { ...config.gitOps, path: e.target.value } })} placeholder="." />
              </label>
            </div>
          )}
          {config.multiCluster.enableMultiCluster && (
            <label>
              Target Clusters
              <input value={config.multiCluster.clusters?.map((cluster) => cluster.name).join(', ') || ''} onChange={(e) => onChange({ ...config, multiCluster: { ...config.multiCluster, clusters: e.target.value.split(',').map((c) => ({ name: c.trim() })) } })} placeholder="cluster1, cluster2" />
            </label>
          )}
        </div>
      )}

      {currentStep === 7 && (
        <div className="wizard-panel">
          <h3>Review Configuration</h3>
          <p>Inspect the generated configuration before deployment. You can edit the YAML manifests in the next step.</p>
          <div className="review-grid">
            <div>
              <strong>Application</strong>
              <p>{config.appName}</p>
            </div>
            <div>
              <strong>Storage</strong>
              <p>{config.storage.storageType.toUpperCase()}</p>
            </div>
            <div>
              <strong>Workload</strong>
              <p>{config.workloadType}</p>
            </div>
            <div>
              <strong>Namespace</strong>
              <p>{config.namespace}</p>
            </div>
            <div>
              <strong>Networking</strong>
              <p>Service: {config.networking.enableService ? 'Yes' : 'No'}, Ingress: {config.networking.enableIngress ? 'Yes' : 'No'}</p>
            </div>
            <div>
              <strong>Security</strong>
              <p>RBAC: {config.security.enableRBAC ? 'Yes' : 'No'}</p>
            </div>
            <div>
              <strong>Monitoring</strong>
              <p>{config.monitoring.monitoring}</p>
            </div>
            <div>
              <strong>Logging</strong>
              <p>{config.logging.logging}</p>
            </div>
          </div>
        </div>
      )}

      <footer className="wizard-actions">
        <button onClick={onBack} disabled={currentStep === 1} type="button">
          Back
        </button>
        <button onClick={onNext} type="button">
          {currentStep === 7 ? 'Generate Manifests' : 'Next'}
        </button>
      </footer>
    </section>
  );
}
