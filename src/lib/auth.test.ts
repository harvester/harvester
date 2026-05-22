import { describe, expect, it } from 'vitest';
import { isDemoLogin } from './auth';

describe('isDemoLogin', () => {
  it('accepts the documented admin demo credentials', () => {
    expect(isDemoLogin('admin', 'demo')).toBe(true);
  });

  it('rejects any non-demo credential combination', () => {
    expect(isDemoLogin('admin', 'wrong')).toBe(false);
    expect(isDemoLogin('user', 'demo')).toBe(false);
    expect(isDemoLogin('', '')).toBe(false);
  });
});
