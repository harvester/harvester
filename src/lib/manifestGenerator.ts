import YAML from 'yaml';
import { ApplicationConfig } from '../types';

const buildLabels = (config: ApplicationConfig) => ({
  app: config.appName,
  tier: 'frontend',
});

export function generateManifest(config: ApplicationConfig): string {
  const manifests: string[] = [];

  // Generate workload manifest
  const workloadManifest = generateWorkloadManifest(config);
  manifests.push(workloadManifest);

  // Generate PVC for the selected storage backend
  const pvcManifest = generatePVCManifest(config);
  manifests.push(pvcManifest);

  // Generate Service if enabled
  if (config.networking.enableService) {
    const serviceManifest = generateServiceManifest(config);
    manifests.push(serviceManifest);
  }

  // Generate Ingress if enabled
  if (config.networking.enableIngress) {
    const ingressManifest = generateIngressManifest(config);
    manifests.push(ingressManifest);
  }

  // Generate NetworkPolicy if enabled
  if (config.networking.enableNetworkPolicy) {
    const networkPolicyManifest = generateNetworkPolicyManifest(config);
    manifests.push(networkPolicyManifest);
  }

  // Generate RBAC if enabled
  if (config.security.enableRBAC) {
    const rbacManifests = generateRBACManifests(config);
    manifests.push(...rbacManifests);
  }

  // Generate Monitoring if enabled
  if (config.monitoring.monitoring !== 'none') {
    const monitoringManifests = generateMonitoringManifests(config);
    manifests.push(...monitoringManifests);
  }

  // Generate Logging if enabled
  if (config.logging.logging !== 'none') {
    const loggingManifests = generateLoggingManifests(config);
    manifests.push(...loggingManifests);
  }

  // Generate GitOps if enabled
  if (config.gitOps.gitOps !== 'none') {
    const gitOpsManifests = generateGitOpsManifests(config);
    manifests.push(...gitOpsManifests);
  }

  // Generate Multi-cluster if enabled
  if (config.multiCluster.enableMultiCluster) {
    const multiClusterManifests = generateMultiClusterManifests(config);
    manifests.push(...multiClusterManifests);
  }

  // Generate Service Mesh if enabled
  if (config.networking.serviceMesh !== 'none') {
    const meshManifests = generateServiceMeshManifests(config);
    manifests.push(...meshManifests);
  }

  return manifests.join('\n---\n');
}

function generateWorkloadManifest(config: ApplicationConfig): string {
  const labels = buildLabels(config);
  const podSpec = {
    containers: [
      {
        name: config.appName,
        image: config.image,
        ports: [{ containerPort: config.networking.servicePort }],
        resources: {
          requests: {
            cpu: config.cpuRequest,
            memory: config.memoryRequest,
          },
          limits: {
            cpu: config.cpuLimit,
            memory: config.memoryLimit,
          },
        },
        env: Object.entries(config.environmentVariables || {}).map(([key, value]) => ({
          name: key,
          value: value,
        })),
        ...(config.enableHealthChecks ? {
          livenessProbe: {
            httpGet: {
              path: config.healthCheckPath,
              port: config.networking.servicePort,
            },
            initialDelaySeconds: 30,
            periodSeconds: 10,
          },
          readinessProbe: {
            httpGet: {
              path: config.healthCheckPath,
              port: config.networking.servicePort,
            },
            initialDelaySeconds: 5,
            periodSeconds: 5,
          },
        } : {}),
        volumeMounts: [
          {
            name: 'app-storage',
            mountPath: '/data',
          },
        ],
      },
    ],
    volumes: [
      {
        name: 'app-storage',
        persistentVolumeClaim: {
          claimName: `${config.appName}-pvc`,
        },
      },
    ],
  };

  const workload = {
    apiVersion: config.workloadType === 'StatefulSet' ? 'apps/v1' : config.workloadType === 'DaemonSet' ? 'apps/v1' : config.workloadType === 'Job' ? 'batch/v1' : config.workloadType === 'CronJob' ? 'batch/v1' : 'apps/v1',
    kind: config.workloadType,
    metadata: {
      name: config.appName,
      namespace: config.namespace,
      labels: { ...labels, ...config.labels },
      annotations: config.annotations,
    },
    spec: {
      ...(config.workloadType === 'Deployment' || config.workloadType === 'StatefulSet'
        ? {
            replicas: config.workloadType === 'Deployment' ? config.replicas : undefined,
            selector: { matchLabels: labels },
            template: { metadata: { labels: { ...labels, ...config.labels }, annotations: config.annotations }, spec: podSpec },
          }
        : {}),
      ...(config.workloadType === 'DaemonSet'
        ? { selector: { matchLabels: labels }, template: { metadata: { labels: { ...labels, ...config.labels }, annotations: config.annotations }, spec: podSpec } }
        : {}),
      ...(config.workloadType === 'Job'
        ? { template: { metadata: { labels: { ...labels, ...config.labels }, annotations: config.annotations }, spec: podSpec }, backoffLimit: 2 }
        : {}),
      ...(config.workloadType === 'CronJob'
        ? {
            schedule: '0 3 * * *',
            jobTemplate: { spec: { template: { metadata: { labels: { ...labels, ...config.labels }, annotations: config.annotations }, spec: podSpec } } },
          }
        : {}),
    },
  };

  return YAML.stringify(workload);
}

function generatePVCManifest(config: ApplicationConfig): string {
  const pvc = {
    apiVersion: 'v1',
    kind: 'PersistentVolumeClaim',
    metadata: {
      name: `${config.appName}-pvc`,
      namespace: config.namespace,
      labels: buildLabels(config),
    },
    spec: {
      accessModes: [config.storage.accessMode],
      resources: {
        requests: {
          storage: config.storage.storageSize,
        },
      },
      storageClassName: config.storage.storageClass,
    },
  };

  return YAML.stringify(pvc);
}

function generateServiceManifest(config: ApplicationConfig): string {
  const service = {
    apiVersion: 'v1',
    kind: 'Service',
    metadata: {
      name: `${config.appName}-svc`,
      namespace: config.namespace,
      labels: buildLabels(config),
    },
    spec: {
      type: config.networking.serviceType,
      selector: buildLabels(config),
      ports: [
        {
          name: 'http',
          port: config.networking.servicePort,
          targetPort: config.networking.servicePort,
        },
      ],
    },
  };

  return YAML.stringify(service);
}

function generateIngressManifest(config: ApplicationConfig): string {
  const ingress = {
    apiVersion: 'networking.k8s.io/v1',
    kind: 'Ingress',
    metadata: {
      name: `${config.appName}-ingress`,
      namespace: config.namespace,
      labels: buildLabels(config),
    },
    spec: {
      rules: [
        {
          host: config.networking.hostname,
          http: {
            paths: [
              {
                path: '/',
                pathType: 'Prefix',
                backend: { service: { name: `${config.appName}-svc`, port: { number: config.networking.servicePort } } },
              },
            ],
          },
        },
      ],
    },
  };

  return YAML.stringify(ingress);
}

function generateNetworkPolicyManifest(config: ApplicationConfig): string {
  const networkPolicy = {
    apiVersion: 'networking.k8s.io/v1',
    kind: 'NetworkPolicy',
    metadata: {
      name: `${config.appName}-network-policy`,
      namespace: config.namespace,
      labels: buildLabels(config),
    },
    spec: {
      podSelector: {
        matchLabels: buildLabels(config),
      },
      policyTypes: ['Ingress', 'Egress'],
      ingress: [
        {
          from: [
            {
              podSelector: {
                matchLabels: {
                  app: config.appName,
                },
              },
            },
          ],
          ports: [
            {
              protocol: 'TCP',
              port: config.networking.servicePort,
            },
          ],
        },
      ],
      egress: [
        {
          to: [],
          ports: [
            {
              protocol: 'TCP',
              port: 53,
            },
            {
              protocol: 'UDP',
              port: 53,
            },
          ],
        },
      ],
    },
  };

  return YAML.stringify(networkPolicy);
}

function generateRBACManifests(config: ApplicationConfig): string[] {
  const manifests: string[] = [];

  const serviceAccount = {
    apiVersion: 'v1',
    kind: 'ServiceAccount',
    metadata: {
      name: `${config.appName}-sa`,
      namespace: config.namespace,
      labels: buildLabels(config),
    },
  };

  const role = {
    apiVersion: 'rbac.authorization.k8s.io/v1',
    kind: 'Role',
    metadata: {
      name: `${config.appName}-role`,
      namespace: config.namespace,
      labels: buildLabels(config),
    },
    rules: [
      {
        apiGroups: [''],
        resources: ['pods', 'services', 'configmaps', 'secrets'],
        verbs: ['get', 'list', 'watch', 'create', 'update', 'patch', 'delete'],
      },
      {
        apiGroups: ['apps'],
        resources: ['deployments', 'statefulsets'],
        verbs: ['get', 'list', 'watch', 'create', 'update', 'patch', 'delete'],
      },
    ],
  };

  const roleBinding = {
    apiVersion: 'rbac.authorization.k8s.io/v1',
    kind: 'RoleBinding',
    metadata: {
      name: `${config.appName}-rolebinding`,
      namespace: config.namespace,
      labels: buildLabels(config),
    },
    subjects: [
      {
        kind: 'ServiceAccount',
        name: `${config.appName}-sa`,
        namespace: config.namespace,
      },
    ],
    roleRef: {
      kind: 'Role',
      name: `${config.appName}-role`,
      apiGroup: 'rbac.authorization.k8s.io',
    },
  };

  manifests.push(YAML.stringify(serviceAccount));
  manifests.push(YAML.stringify(role));
  manifests.push(YAML.stringify(roleBinding));

  return manifests;
}

function generateMonitoringManifests(config: ApplicationConfig): string[] {
  const manifests: string[] = [];

  if (config.monitoring.monitoring === 'prometheus') {
    const serviceMonitor = {
      apiVersion: 'monitoring.coreos.com/v1',
      kind: 'ServiceMonitor',
      metadata: {
        name: `${config.appName}-servicemonitor`,
        namespace: config.namespace,
        labels: buildLabels(config),
      },
      spec: {
        selector: {
          matchLabels: buildLabels(config),
        },
        endpoints: [
          {
            port: 'http',
            path: config.monitoring.metricsPath || '/metrics',
            interval: '30s',
          },
        ],
      },
    };

    manifests.push(YAML.stringify(serviceMonitor));
  }

  if (config.monitoring.enableMetrics) {
    const configMap = {
      apiVersion: 'v1',
      kind: 'ConfigMap',
      metadata: {
        name: `${config.appName}-monitoring-config`,
        namespace: config.namespace,
        labels: buildLabels(config),
      },
      data: {
        'prometheus.yml': `
global:
  scrape_interval: 15s
scrape_configs:
  - job_name: '${config.appName}'
    static_configs:
      - targets: ['${config.appName}-svc:${config.networking.servicePort}']
`,
      },
    };

    manifests.push(YAML.stringify(configMap));
  }

  return manifests;
}

function generateLoggingManifests(config: ApplicationConfig): string[] {
  const manifests: string[] = [];

  if (config.logging.logging === 'fluentd') {
    const configMap = {
      apiVersion: 'v1',
      kind: 'ConfigMap',
      metadata: {
        name: `${config.appName}-fluentd-config`,
        namespace: config.namespace,
        labels: buildLabels(config),
      },
      data: {
        'fluent.conf': `
<source>
  @type tail
  path /var/log/containers/*${config.appName}*.log
  pos_file /var/log/fluentd-${config.appName}.pos
  tag ${config.appName}
  <parse>
    @type json
  </parse>
</source>
<match ${config.appName}>
  @type elasticsearch
  host elasticsearch
  port 9200
  logstash_format true
  logstash_prefix ${config.appName}
</match>
`,
      },
    };

    manifests.push(YAML.stringify(configMap));
  }

  if (config.logging.logging === 'loki') {
    const configMap = {
      apiVersion: 'v1',
      kind: 'ConfigMap',
      metadata: {
        name: `${config.appName}-loki-config`,
        namespace: config.namespace,
        labels: buildLabels(config),
      },
      data: {
        'loki.yaml': `
server:
  http_listen_port: 3100
positions:
  filename: /tmp/positions.yaml
clients:
  - url: http://loki:3100/loki/api/v1/push
scrape_configs:
  - job_name: ${config.appName}
    static_configs:
      - targets:
          - localhost
    pipeline_stages:
      - docker: {}
`,
      },
    };

    manifests.push(YAML.stringify(configMap));
  }

  if (config.logging.logging === 'splunk') {
    const configMap = {
      apiVersion: 'v1',
      kind: 'ConfigMap',
      metadata: {
        name: `${config.appName}-splunk-config`,
        namespace: config.namespace,
        labels: buildLabels(config),
      },
      data: {
        'outputs.conf': `[tcpout]
defaultGroup = default-autolb-group
[tcpout:default-autolb-group]
server = splunk:9997
`,
      },
    };

    manifests.push(YAML.stringify(configMap));
  }

  if (config.logging.logging === 'elasticsearch') {
    const configMap = {
      apiVersion: 'v1',
      kind: 'ConfigMap',
      metadata: {
        name: `${config.appName}-elastic-config`,
        namespace: config.namespace,
        labels: buildLabels(config),
      },
      data: {
        'logstash.conf': `
input {
  file {
    path => "/var/log/containers/*${config.appName}*.log"
    start_position => "beginning"
  }
}
output {
  elasticsearch {
    hosts => ["elasticsearch:9200"]
    index => "${config.appName}-%{+YYYY.MM.dd}"
  }
}
`,
      },
    };

    manifests.push(YAML.stringify(configMap));
  }

  return manifests;
}

function generateGitOpsManifests(config: ApplicationConfig): string[] {
  const manifests: string[] = [];

  if (config.gitOps.gitOps === 'argocd') {
    const application = {
      apiVersion: 'argoproj.io/v1alpha1',
      kind: 'Application',
      metadata: {
        name: config.appName,
        namespace: 'argocd',
        labels: buildLabels(config),
      },
      spec: {
        project: 'default',
        source: {
          repoURL: config.gitOps.repoUrl || config.gitOps.gitRepository || 'https://github.com/example/repo',
          targetRevision: config.gitOps.gitBranch || 'HEAD',
          path: config.gitOps.path || '.',
        },
        destination: {
          server: 'https://kubernetes.default.svc',
          namespace: config.namespace,
        },
        syncPolicy: {
          automated: {
            prune: config.gitOps.prune ?? true,
            selfHeal: config.gitOps.autoSync ?? true,
          },
        },
      },
    };

    manifests.push(YAML.stringify(application));
  }

  if (config.gitOps.gitOps === 'flux') {
    const fluxSource = {
      apiVersion: 'source.toolkit.fluxcd.io/v1beta2',
      kind: 'GitRepository',
      metadata: {
        name: `${config.appName}-repo`,
        namespace: config.namespace,
        labels: buildLabels(config),
      },
      spec: {
        interval: `${config.gitOps.syncInterval || 5}m`,
        url: config.gitOps.repoUrl || config.gitOps.gitRepository || 'https://github.com/example/repo',
        ref: {
          branch: config.gitOps.gitBranch || 'main',
        },
      },
    };

    const kustomization = {
      apiVersion: 'kustomize.toolkit.fluxcd.io/v1beta2',
      kind: 'Kustomization',
      metadata: {
        name: `${config.appName}-kustomization`,
        namespace: config.namespace,
        labels: buildLabels(config),
      },
      spec: {
        interval: `${config.gitOps.syncInterval || 5}m`,
        path: config.gitOps.path || '.',
        prune: config.gitOps.prune ?? true,
        sourceRef: {
          kind: 'GitRepository',
          name: `${config.appName}-repo`,
        },
        targetNamespace: config.namespace,
      },
    };

    manifests.push(YAML.stringify(fluxSource));
    manifests.push(YAML.stringify(kustomization));
  }

  if (config.gitOps.gitOps === 'jenkinsx') {
    const pipeline = {
      apiVersion: 'jenkins.io/v1alpha2',
      kind: 'PipelineActivity',
      metadata: {
        name: `${config.appName}-pipeline`,
        namespace: config.namespace,
        labels: buildLabels(config),
      },
      spec: {
        gitOwner: config.gitOps.gitRepository ? config.gitOps.gitRepository.split('/')[3] : 'example',
        gitRepository: config.gitOps.gitRepository ? config.gitOps.gitRepository.split('/')[4] : 'repo',
        branch: config.gitOps.gitBranch || 'main',
        build: '1',
      },
    };

    manifests.push(YAML.stringify(pipeline));
  }

  return manifests;
}

function generateServiceMeshManifests(config: ApplicationConfig): string[] {
  const manifests: string[] = [];

  if (config.networking.serviceMesh === 'istio') {
    manifests.push(`apiVersion: networking.istio.io/v1beta1
kind: Gateway
metadata:
  name: ${config.appName}-gateway
  namespace: ${config.namespace}
spec:
  selector:
    istio: ingressgateway
  servers:
  - port:
      number: ${config.networking.servicePort}
      name: http
      protocol: HTTP
    hosts:
    - "${config.networking.hostname || '*'}"`);
  }

  if (config.networking.serviceMesh === 'linkerd') {
    manifests.push(`apiVersion: linkerd.io/v1alpha1
kind: LinkerdServiceProfile
metadata:
  name: ${config.appName}.${config.namespace}.svc.cluster.local
  namespace: ${config.namespace}
spec:
  routes:
  - name: http
    condition:
      method: GET
    responseClasses:
    - condition:
        status:
          min: 200
          max: 399`);
  }

  if (config.networking.serviceMesh === 'cilium') {
    manifests.push(`apiVersion: cilium.io/v2
kind: CiliumNetworkPolicy
metadata:
  name: ${config.appName}-cilium-policy
  namespace: ${config.namespace}
spec:
  endpointSelector:
    matchLabels:
      app: ${config.appName}
  ingress:
  - toPorts:
    - ports:
      - port: ${config.networking.servicePort}
        protocol: TCP`);
  }

  return manifests;
}

function generateMultiClusterManifests(config: ApplicationConfig): string[] {
  const manifests: string[] = [];

  if (config.multiCluster.enableMultiCluster) {
    const federatedDeployment = {
      apiVersion: 'types.kubefed.io/v1beta1',
      kind: 'FederatedDeployment',
      metadata: {
        name: config.appName,
        namespace: config.namespace,
        labels: buildLabels(config),
      },
      spec: {
        template: {
          metadata: {
            labels: buildLabels(config),
          },
          spec: {
            replicas: config.replicas,
            selector: {
              matchLabels: buildLabels(config),
            },
            template: {
              metadata: {
                labels: buildLabels(config),
              },
              spec: {
                containers: [
                  {
                    name: config.appName,
                    image: config.image,
                    ports: [{ containerPort: config.networking.servicePort }],
                  },
                ],
              },
            },
          },
        },
        placement: {
          clusters: config.multiCluster.clusters || [
            { name: 'cluster1' },
            { name: 'cluster2' },
          ],
        },
      },
    };

    manifests.push(YAML.stringify(federatedDeployment));
  }

  return manifests;
}
