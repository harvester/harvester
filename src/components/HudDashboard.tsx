import type { CSSProperties } from 'react';
import { buildHudTelemetry } from '../lib/hudTelemetry';

const telemetry = buildHudTelemetry();
const linePoints = telemetry.lineSeries
  .map((value, index) => `${(index / (telemetry.lineSeries.length - 1)) * 100},${100 - value}`)
  .join(' ');

export function HudDashboard() {
  return (
    <section className="hud-dashboard" aria-label="Animated Nexus cluster dashboard mockup">
      <div className="hud-scanlines" />
      <div className="hud-orb hud-orb-left" />
      <div className="hud-orb hud-orb-right" />

      <div className="hud-hero hud-panel">
        <div>
          <span className="hud-kicker">NEXUS // HARVESTER CONTROL</span>
          <h2>Live cluster command surface</h2>
          <p>
            Animated telemetry mockup for validation, apply readiness, storage health, service mesh, and multi-cluster targeting.
          </p>
        </div>
        <div className="hud-status-pill">
          <span className="hud-live-dot" />
          DEMO STREAM ACTIVE
        </div>
      </div>

      <div className="hud-reference-controls hud-panel">
        <nav className="hud-segment-menu" aria-label="HUD dashboard menu modes">
          {telemetry.menuModes.map((mode, index) => (
            <button className={index === 0 ? 'is-selected' : ''} key={mode} type="button">
              {mode}
            </button>
          ))}
        </nav>
        <label className="hud-select-shell">
          <span>Target cluster</span>
          <select value="edge-a-vcluster" onChange={() => undefined}>
            <option value="edge-a-vcluster">edge-a / vcluster</option>
            <option value="edge-b-vcluster">edge-b / vcluster</option>
            <option value="control-plane">control-plane</option>
          </select>
        </label>
      </div>

      <div className="hud-metric-grid">
        {telemetry.metrics.map((metric) => (
          <article className={`hud-panel hud-metric hud-status-${metric.status}`} key={metric.label}>
            <div className="hud-metric-header">
              <span>{metric.label}</span>
              <strong>{metric.trend}</strong>
            </div>
            <div className="hud-metric-value">
              {metric.value}
              <span>{metric.unit}</span>
            </div>
            <div className="hud-meter" aria-hidden="true">
              <span style={{ width: `${metric.value}%` }} />
            </div>
          </article>
        ))}
      </div>

      <div className="hud-visual-grid">
        <article className="hud-panel hud-radar">
          <div className="hud-panel-title">
            <span>Topology pulse</span>
            <strong>4 nodes</strong>
          </div>
          <div className="hud-radar-map">
            <div className="hud-radar-ring ring-one" />
            <div className="hud-radar-ring ring-two" />
            <svg viewBox="0 0 100 100" aria-hidden="true">
              <polyline points="50,20 22,58 50,82 78,58 50,20" />
              <line x1="22" y1="58" x2="78" y2="58" />
              <line x1="50" y1="20" x2="50" y2="82" />
            </svg>
            {telemetry.nodes.map((node) => (
              <span
                className={`hud-node hud-node-${node.status}`}
                key={node.id}
                style={{ left: `${node.x}%`, top: `${node.y}%` }}
              >
                <i />
                <b>{node.label}</b>
              </span>
            ))}
          </div>
        </article>

        <article className="hud-panel hud-storage">
          <div className="hud-panel-title">
            <span>CSI storage rings</span>
            <strong>green path</strong>
          </div>
          <div className="hud-rings">
            {telemetry.storageRings.map((ring) => (
              <div className="hud-ring" key={ring.label} style={{ '--ring-value': `${ring.value * 3.6}deg` } as CSSProperties}>
                <div className="hud-ring-core">
                  <strong>{ring.value}%</strong>
                  <span>{ring.label}</span>
                </div>
              </div>
            ))}
          </div>
        </article>

        <article className="hud-panel hud-throughput">
          <div className="hud-panel-title">
            <span>Manifest apply wave</span>
            <strong>live preview</strong>
          </div>
          <div className="hud-bars">
            {telemetry.throughputBars.map((bar, index) => (
              <span key={`${bar}-${index}`} style={{ height: `${bar}%`, animationDelay: `${index * 90}ms` }} />
            ))}
          </div>
          <div className="hud-data-ribbon">
            <span>validate</span>
            <span>dry-run</span>
            <span>diff</span>
            <span>apply</span>
          </div>
        </article>

        <article className="hud-panel hud-waveform">
          <div className="hud-panel-title">
            <span>Resource waveform</span>
            <strong>sync trace</strong>
          </div>
          <svg viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true">
            <defs>
              <linearGradient id="hudWaveGradient" x1="0%" x2="100%">
                <stop offset="0%" stopColor="#33f7ff" />
                <stop offset="100%" stopColor="#75ff6a" />
              </linearGradient>
            </defs>
            <polyline points={linePoints} />
            {telemetry.lineSeries.map((value, index) => (
              <circle
                key={`${value}-${index}`}
                cx={(index / (telemetry.lineSeries.length - 1)) * 100}
                cy={100 - value}
                r="1.7"
                style={{ animationDelay: `${index * 120}ms` }}
              />
            ))}
          </svg>
        </article>

        <article className="hud-panel hud-toggle-bank">
          <div className="hud-panel-title">
            <span>Control toggles</span>
            <strong>armed</strong>
          </div>
          <div className="hud-toggle-grid">
            {telemetry.toggles.map((toggle) => (
              <div className={toggle.enabled ? 'hud-toggle is-on' : 'hud-toggle'} key={toggle.label}>
                <span>{toggle.label}</span>
                <i />
              </div>
            ))}
          </div>
        </article>

        <article className="hud-panel hud-feed">
          <div className="hud-panel-title">
            <span>Event stream</span>
            <strong>5 signals</strong>
          </div>
          <ul>
            {telemetry.eventFeed.map((event) => (
              <li key={event}>
                <span />
                {event}
              </li>
            ))}
          </ul>
        </article>
      </div>
    </section>
  );
}
