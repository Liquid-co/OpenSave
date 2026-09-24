import { describe, expect, it } from 'vitest';
import { DEFAULT_NOTIFY, loadNotify, mayInterrupt, sanitizeNotify, saveNotify } from './notifyprefs.js';

const memoryStorage = () => {
  const data = {};
  return { getItem: (k) => (k in data ? data[k] : null), setItem: (k, v) => (data[k] = String(v)) };
};
const busy = (yes) => async () => yes;

describe('notification preferences', () => {
  it('keeps booleans it knows and defaults the rest', () => {
    expect(sanitizeNotify({ pairing: false, conflicts: 'no', bogus: true })).toEqual({ ...DEFAULT_NOTIFY, pairing: false });
    expect(sanitizeNotify(null)).toEqual(DEFAULT_NOTIFY);
  });

  it('round-trips through storage and survives a broken one', () => {
    const s = memoryStorage();
    saveNotify({ ...DEFAULT_NOTIFY, newGames: false }, s);
    expect(loadNotify(s).newGames).toBe(false);
    const broken = { getItem: () => { throw new Error('x'); }, setItem: () => { throw new Error('x'); } };
    expect(loadNotify(broken)).toEqual(DEFAULT_NOTIFY);
    expect(() => saveNotify(DEFAULT_NOTIFY, broken)).not.toThrow();
  });
});

describe('mayInterrupt', () => {
  it('respects an event being turned off', async () => {
    expect(await mayInterrupt('pairing', busy(false), { ...DEFAULT_NOTIFY, pairing: false })).toBe(false);
    expect(await mayInterrupt('pairing', busy(false), DEFAULT_NOTIFY)).toBe(true);
  });

  it('holds the chime and the window back during a full-screen game, when asked to', async () => {
    expect(await mayInterrupt('conflicts', busy(true), DEFAULT_NOTIFY)).toBe(false);
    expect(await mayInterrupt('conflicts', busy(true), { ...DEFAULT_NOTIFY, quietWhilePlaying: false })).toBe(true);
  });

  it('does not hold back a corner message — only what takes the screen', async () => {
    expect(await mayInterrupt('cloudPulled', busy(true), DEFAULT_NOTIFY)).toBe(true);
  });

  it('interrupts when it cannot tell whether a game is running', async () => {
    const failing = async () => {
      throw new Error('no native bridge');
    };
    expect(await mayInterrupt('pairing', failing, DEFAULT_NOTIFY)).toBe(true);
  });
});
