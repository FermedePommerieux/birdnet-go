import { describe, expect, it } from 'vitest';
import { createDetectionRateTimeFormatter } from './detectionRateTime';

describe('createDetectionRateTimeFormatter', () => {
  it('distinguishes the repeated autumn hour by its UTC offset', () => {
    const format = createDetectionRateTimeFormatter('Europe/Paris', 'en-GB');
    const first = format('2026-10-25T00:00:00Z');
    const second = format('2026-10-25T01:00:00Z');

    expect(first).toContain('02:00');
    expect(second).toContain('02:00');
    expect(first).toContain('GMT+2');
    expect(second).toContain('GMT+1');
    expect(first).not.toBe(second);
  });

  it('shows the spring clock jump without inventing a local 02:00 hour', () => {
    const format = createDetectionRateTimeFormatter('Europe/Paris', 'en-GB');
    const before = format('2026-03-29T00:00:00Z');
    const after = format('2026-03-29T01:00:00Z');

    expect(before).toContain('01:00');
    expect(before).toContain('GMT+1');
    expect(after).toContain('03:00');
    expect(after).toContain('GMT+2');
  });
});
