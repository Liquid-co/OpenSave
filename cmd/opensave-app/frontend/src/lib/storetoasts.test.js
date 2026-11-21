// The toasts the daemon's messages raise (lib/stores.js).
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { get } from 'svelte/store';
import { applyMessage, toasts, toast } from './stores.js';
import { restoreKey } from './toasts.js';

const shown = () => get(toasts);
const byKey = (key) => shown().find((t) => t.key === key);

describe('toasts from the daemon', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => {
    for (const t of shown()) toasts.remove(t.id);
    vi.useRealTimers();
  });

  it("follows a restore in one toast, which the request's answer finishes", () => {
    const game = { id: 'hades', name: 'Hades', restoring: { phase: 'writing', done: 1, total: 4 } };
    applyMessage({ type: 'games-update', data: { hades: game } });
    const first = byKey(restoreKey('hades'));
    expect(first).toMatchObject({ kind: 'progress', title: 'Restoring Hades', progress: { done: 1, total: 4 } });

    applyMessage({ type: 'games-update', data: { hades: { ...game, restoring: { phase: 'writing', done: 3, total: 4 } } } });
    expect(shown().filter((t) => t.key === restoreKey('hades'))).toHaveLength(1);
    expect(byKey(restoreKey('hades')).progress).toEqual({ done: 3, total: 4 });

    // Done: the daemon stops reporting it, and the answer says so.
    applyMessage({ type: 'games-update', data: { hades: { ...game, restoring: null } } });
    toast('Snapshot restored', 'success', { key: restoreKey('hades'), progress: null });
    const done = byKey(restoreKey('hades'));
    expect(done).toMatchObject({ id: first.id, kind: 'success', title: 'Snapshot restored', progress: null });
  });

  it('finishes a restore toast whose answer never comes here', () => {
    const game = { id: 'celeste', name: 'Celeste', restoring: { phase: 'writing', done: 1, total: 2 } };
    applyMessage({ type: 'games-update', data: { celeste: game } });
    applyMessage({ type: 'games-update', data: { celeste: { ...game, restoring: null } } });
    expect(byKey(restoreKey('celeste')).kind).toBe('progress');
    vi.advanceTimersByTime(3001);
    expect(byKey(restoreKey('celeste'))).toMatchObject({ kind: 'success', title: 'Celeste restored' });
  });

  it('leaves a failed restore as the failure', () => {
    const game = { id: 'cuphead', name: 'Cuphead', restoring: { phase: 'writing', done: 1, total: 2 } };
    applyMessage({ type: 'games-update', data: { cuphead: game } });
    applyMessage({ type: 'games-update', data: { cuphead: { ...game, restoring: null } } });
    toast('the disk is full', 'error', { key: restoreKey('cuphead') });
    vi.advanceTimersByTime(3001);
    expect(byKey(restoreKey('cuphead'))).toMatchObject({ kind: 'error', title: 'The disk is full' });
  });

  it('says a failed sync once per game, offers to try now, and takes it back when a sync works', () => {
    applyMessage({ type: 'games-update', data: { tunic: { id: 'tunic', name: 'Tunic' } } });
    applyMessage({ type: 'sync-error', data: { gameId: 'tunic', data: { error: 'connection reset' } } });
    const failed = byKey('sync-error:tunic');
    expect(failed).toMatchObject({ kind: 'error', title: "Couldn't sync Tunic", game: 'tunic' });
    expect(failed.detail).toMatch(/^The connection to the other device dropped\. It tries again on its own\.$/);
    expect(typeof failed.retry).toBe('function');

    // The retry loop fails again: the same toast, not another.
    applyMessage({ type: 'sync-error', data: { gameId: 'tunic', data: { error: 'connection reset' } } });
    expect(shown().filter((t) => t.key === 'sync-error:tunic')).toHaveLength(1);

    applyMessage({ type: 'sync-complete', data: { gameId: 'tunic', data: {} } });
    expect(byKey('sync-error:tunic')).toMatchObject({ id: failed.id, kind: 'success', title: 'Tunic synced', retry: null });
  });

  it('shows files that came in as what came, over the game', () => {
    applyMessage({ type: 'games-update', data: { hk: { id: 'hk', name: 'Hollow Knight' } } });
    applyMessage({ type: 'activity', data: { kind: 'received', gameId: 'hk', files: 3, device: 'Steam Deck' } });
    expect(shown().at(-1)).toMatchObject({ kind: 'received', title: 'Got 3 files from Steam Deck', detail: 'Hollow Knight', game: 'hk' });
  });
});
