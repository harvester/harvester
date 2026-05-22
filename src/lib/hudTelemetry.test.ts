import { describe, expect, it } from 'vitest';
import { buildHudTelemetry } from './hudTelemetry';

describe('buildHudTelemetry', () => {
  it('returns active dashboard telemetry for the default Nexus demo', () => {
    const telemetry = buildHudTelemetry();

    expect(telemetry.metrics).toHaveLength(4);
    expect(telemetry.metrics.every((metric) => metric.value >= 0 && metric.value <= 100)).toBe(true);
    expect(telemetry.storageRings.map((ring) => ring.label)).toEqual(['Ceph', 'Longhorn', 'NVMe-oF']);
    expect(telemetry.eventFeed[0]).toContain('control-plane');
  });
});
