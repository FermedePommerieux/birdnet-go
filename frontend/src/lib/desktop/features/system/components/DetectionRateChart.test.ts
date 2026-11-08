import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup } from '@testing-library/svelte';
import { renderTyped } from '../../../../../test/render-helpers';
import DetectionRateChart from './DetectionRateChart.svelte';

describe('DetectionRateChart', () => {
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it('shows distinct offsets for the repeated local hour in bar tooltips', () => {
    // Set the simulated browser timezone, independent of the Vitest worker's TZ.
    const DateTimeFormat = Intl.DateTimeFormat;
    vi.spyOn(Intl, 'DateTimeFormat').mockImplementation(
      function DateTimeFormatWithParis(locales, options) {
        return new DateTimeFormat(locales, {
          ...options,
          timeZone: options?.timeZone ?? 'Europe/Paris',
        });
      }
    );
    const { container } = renderTyped(DetectionRateChart, {
      props: {
        data: [
          { hour: '2026-10-25T00:00:00Z', count: 2 },
          { hour: '2026-10-25T01:00:00Z', count: 3 },
        ],
      },
    });

    const titles = Array.from(container.querySelectorAll('rect title'), title => title.textContent);
    expect(titles).toHaveLength(2);
    expect(titles[0]).toContain('02:00');
    expect(titles[0]).toContain('GMT+2');
    expect(titles[1]).toContain('02:00');
    expect(titles[1]).toContain('GMT+1');
    expect(titles[0]).not.toBe(titles[1]);
  });
});
