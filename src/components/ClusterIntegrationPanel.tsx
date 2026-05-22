import { ApplicationConfig } from '../types';
import {
  ApplyTestRun,
  CsiTemplatePreview,
  LivePreview,
  ValidationResult,
  VClusterPlan,
} from '../lib/clusterWorkflow';

interface ClusterIntegrationPanelProps {
  validation: ValidationResult;
  livePreview: LivePreview;
  applyRun: ApplyTestRun;
  vclusterPlan: VClusterPlan;
  csiPreview: CsiTemplatePreview;
  config: ApplicationConfig;
}

export function ClusterIntegrationPanel({
  validation,
  livePreview,
  applyRun,
  vclusterPlan,
  csiPreview,
  config,
}: ClusterIntegrationPanelProps) {
  return (
    <section className="cluster-console hud-panel" aria-label="Nexus next steps cluster integration console">
      <div className="hud-panel-title cluster-console-title">
        <span>README next steps console</span>
        <strong>{applyRun.status === 'passed' ? 'all demo checks green' : 'attention required'}</strong>
      </div>

      <div className="cluster-step-grid">
        <article className="cluster-step-card cluster-validation">
          <div className="cluster-card-heading">
            <span>01</span>
            <h3>Kubernetes API validation + live preview</h3>
          </div>
          <div className={`cluster-score ${validation.valid ? 'is-green' : 'is-warn'}`}>
            {livePreview.readinessScore}
            <span>%</span>
          </div>
          <p>{livePreview.summary}</p>
          <ul>
            {livePreview.resources.slice(0, 5).map((resource) => (
              <li key={`${resource.kind}-${resource.name}`}>
                <span />
                {resource.kind}/{resource.name}
              </li>
            ))}
          </ul>
          {validation.issues.length > 0 && (
            <div className="cluster-issues">
              {validation.issues.map((issue) => (
                <p key={`${issue.resource}-${issue.message}`}>{issue.severity.toUpperCase()}: {issue.message}</p>
              ))}
            </div>
          )}
        </article>

        <article className="cluster-step-card">
          <div className="cluster-card-heading">
            <span>02</span>
            <h3>kubectl apply / test runner</h3>
          </div>
          <div className="cluster-checks">
            {applyRun.checks.map((check) => (
              <div className={check.passed ? 'cluster-check passed' : 'cluster-check'} key={check.label}>
                <i />
                <div>
                  <strong>{check.label}</strong>
                  <small>{check.detail}</small>
                </div>
              </div>
            ))}
          </div>
          <code>{applyRun.commands[0]}</code>
        </article>

        <article className="cluster-step-card cluster-vcluster">
          <div className="cluster-card-heading">
            <span>03</span>
            <h3>vcluster / multi-cluster</h3>
          </div>
          <div className="vcluster-map">
            {vclusterPlan.virtualClusters.map((cluster, index) => (
              <div className="vcluster-node" key={cluster.name} style={{ animationDelay: `${index * 180}ms` }}>
                <span>{cluster.name}</span>
                <small>{cluster.context}</small>
              </div>
            ))}
          </div>
          <p>{vclusterPlan.serviceDiscovery}</p>
          <code>{vclusterPlan.commands[0]}</code>
        </article>

        <article className="cluster-step-card cluster-editor">
          <div className="cluster-card-heading">
            <span>04</span>
            <h3>CodeMirror YAML editor</h3>
          </div>
          <div className="editor-capabilities">
            <span>syntax highlighting</span>
            <span>line numbers</span>
            <span>dark HUD theme</span>
            <span>validation-fed preview</span>
          </div>
          <p>The manifest panel now uses CodeMirror YAML editing and drives validation, preview, and test runner output.</p>
        </article>

        <article className="cluster-step-card cluster-csi">
          <div className="cluster-card-heading">
            <span>05</span>
            <h3>Real CSI storage templates</h3>
          </div>
          <div className="csi-driver-pill">{csiPreview.driverName}</div>
          <div className="csi-template-list">
            {csiPreview.templates.map((template) => (
              <details key={`${template.kind}-${template.name}`} open={template.kind === 'StorageClass'}>
                <summary>{template.kind}: {template.name}</summary>
                <pre>{template.yaml}</pre>
              </details>
            ))}
          </div>
        </article>
      </div>

      <div className="cluster-command-ribbon">
        <span>target namespace: {config.namespace}</span>
        <span>workload: {config.workloadType}/{config.appName}</span>
        <span>storage class: {csiPreview.storageClassName}</span>
      </div>
    </section>
  );
}
