import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import { hiddenKeys, withUndo } from './undo.js';

// A notify that records toasts and lets the test press their buttons.
const recorder = () => {
  const toasts = [];
  const notify = (message, kind, opts = {}) => toasts.push({ message, kind, ...opts });
  return { toasts, notify, undo: () => toasts.find((t) => t.action)?.action.run() };
};

describe('withUndo', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => {
    vi.useRealTimers();
    hiddenKeys.set(new Set());
  });

  it('hides at once and carries the action out after the delay', async () => {
    const r = recorder();
    const run = vi.fn(async () => {});
    const outcome = withUndo({ message: 'Stopped tracking Hades', keys: ['g1'], run, delay: 1000, notify: r.notify });

    expect(get(hiddenKeys).has('g1')).toBe(true);
    expect(r.toasts[0]).toMatchObject({ message: 'Stopped tracking Hades', ttl: 1000 });
    expect(r.toasts[0].action.label).toBe('Undo');
    expect(run).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(1000);
    expect(run).toHaveBeenCalledOnce();
    expect(await outcome).toBe('done');
  });

  it('never runs the action when undone, and shows the thing again', async () => {
    const r = recorder();
    const run = vi.fn(async () => {});
    const outcome = withUndo({ message: 'x', keys: ['g1'], run, delay: 1000, notify: r.notify });

    r.undo();
    expect(await outcome).toBe('undone');
    expect(get(hiddenKeys).has('g1')).toBe(false);
    await vi.advanceTimersByTimeAsync(5000);
    expect(run).not.toHaveBeenCalled();
  });

  it('shows the thing again and says why when the action fails', async () => {
    const r = recorder();
    const outcome = withUndo({ message: 'x', keys: ['g1'], run: async () => { throw new Error('daemon said no'); }, delay: 10, notify: r.notify });
    await vi.advanceTimersByTimeAsync(10);
    expect(await outcome).toBe('failed');
    expect(get(hiddenKeys).has('g1')).toBe(false);
    expect(r.toasts.at(-1)).toMatchObject({ message: 'daemon said no', kind: 'error' });
  });

  it('keeps a done thing hidden until the update removes it, so it does not flicker back', async () => {
    const r = recorder();
    let listed = true;
    const outcome = withUndo({ message: 'x', keys: ['g1'], run: async () => {}, delay: 10, notify: r.notify, stillThere: () => listed });
    await vi.advanceTimersByTimeAsync(10);
    expect(await outcome).toBe('done');
    expect(get(hiddenKeys).has('g1')).toBe(true);

    listed = false;
    await vi.advanceTimersByTimeAsync(300);
    expect(get(hiddenKeys).has('g1')).toBe(false);
  });

  it('does not keep a key hidden for good if the update never comes', async () => {
    const r = recorder();
    const outcome = withUndo({ message: 'x', keys: ['g1'], run: async () => {}, delay: 10, notify: r.notify, stillThere: () => true });
    await vi.advanceTimersByTimeAsync(10);
    await outcome;
    await vi.advanceTimersByTimeAsync(6000);
    expect(get(hiddenKeys).has('g1')).toBe(false);
  });

  it('ignores Undo pressed after the action already ran', async () => {
    const r = recorder();
    const run = vi.fn(async () => {});
    const outcome = withUndo({ message: 'x', keys: ['g1'], run, delay: 10, notify: r.notify, stillThere: () => true });
    await vi.advanceTimersByTimeAsync(10);
    r.undo();
    expect(await outcome).toBe('done');
    expect(run).toHaveBeenCalledOnce();
    // Still on its way out: a late Undo must not bring back what is gone.
    expect(get(hiddenKeys).has('g1')).toBe(true);
  });
});
