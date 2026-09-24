import { describe, expect, it } from 'vitest';
import { get } from 'svelte/store';
import { pauseLength, pauseShort } from './syncpause.js';
import { applyMessage, syncPause } from './stores.js';

const now = Date.parse('2026-09-25T12:00:00Z');
const endingIn = (minutes) => ({ paused: true, endsAt: now + minutes * 60000 });

describe('pause wording', () => {
  it('says nothing when not paused', () => {
    expect(pauseLength({ paused: false }, now)).toBe('');
    expect(pauseShort(null, now)).toBe('');
  });

  it('says how long is left, rounding up to the minute', () => {
    expect(pauseLength(endingIn(42), now)).toBe('for 42 more minutes');
    expect(pauseLength(endingIn(41.2), now)).toBe('for 42 more minutes');
    expect(pauseLength(endingIn(65), now)).toBe('for 1 h 05 min');
    expect(pauseLength(endingIn(120), now)).toBe('for 2 h');
    expect(pauseLength(endingIn(0.5), now)).toBe('for less than a minute');
    expect(pauseShort(endingIn(42), now)).toBe('42 min left');
    expect(pauseShort(endingIn(90), now)).toBe('1 h 30 min left');
  });

  it('has no end for a pause until resumed', () => {
    expect(pauseLength({ paused: true, untilRestart: true }, now)).toBe('until you resume');
    expect(pauseShort({ paused: true, untilRestart: true }, now)).toBe('until resumed');
  });
});

describe('the pause store', () => {
  it('follows the daemon: the first state, then every change', () => {
    applyMessage({ type: 'init', data: { games: {}, syncPause: { paused: true, remainingSeconds: 600 } } });
    const p = get(syncPause);
    expect(p.paused).toBe(true);
    expect(p.endsAt - Date.now()).toBeGreaterThan(590000);

    applyMessage({ type: 'sync-pause', data: { paused: true, untilRestart: true } });
    expect(get(syncPause)).toMatchObject({ paused: true, untilRestart: true, endsAt: null });

    applyMessage({ type: 'sync-pause', data: { paused: false } });
    expect(get(syncPause).paused).toBe(false);
  });
});
