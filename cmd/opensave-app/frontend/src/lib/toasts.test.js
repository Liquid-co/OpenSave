import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { get } from 'svelte/store';
import { createToasts, splitMessage, lifeOf, TOAST_LIFE } from './toasts.js';

describe('splitMessage', () => {
  it('puts what follows the first dash on the second line', () => {
    expect(splitMessage('Update failed — the disk is full. Nothing changed — really')).toEqual({
      title: 'Update failed',
      detail: 'The disk is full. Nothing changed — really'
    });
  });
  it('splits the Chinese double dash too', () => {
    expect(splitMessage('同步失败——连接已断开')).toEqual({ title: '同步失败', detail: '连接已断开' });
  });
  it("starts each line with a capital, as the daemon's messages do not", () => {
    expect(splitMessage('the save folder is locked')).toEqual({ title: 'The save folder is locked', detail: '' });
  });
  it('leaves a message without one whole', () => {
    expect(splitMessage('Room code copied')).toEqual({ title: 'Room code copied', detail: '' });
    // A hyphen in a word is not the dash.
    expect(splitMessage('Re-synced Hades-II')).toEqual({ title: 'Re-synced Hades-II', detail: '' });
  });
});

describe('lifeOf', () => {
  it('lets errors and things under way stay; the rest go after five seconds', () => {
    expect(lifeOf('success')).toBe(TOAST_LIFE);
    expect(lifeOf('info')).toBe(TOAST_LIFE);
    expect(lifeOf('error')).toBe(Infinity);
    expect(lifeOf('progress')).toBe(Infinity);
    expect(lifeOf('error', 9000)).toBe(9000);
  });
});

describe('createToasts', () => {
  let toasts;
  beforeEach(() => {
    vi.useFakeTimers();
    toasts = createToasts({ setTimer: setTimeout, clearTimer: clearTimeout, now: () => Date.now() });
  });
  afterEach(() => vi.useRealTimers());
  const shown = () => get(toasts);

  it('lets a done toast go after five seconds, and keeps an error', () => {
    toasts.show('Snapshot taken', 'success');
    toasts.show("Couldn't sync — the Deck went offline", 'error');
    vi.advanceTimersByTime(TOAST_LIFE + 1);
    expect(shown().map((t) => t.kind)).toEqual(['error']);
    expect(shown()[0]).toMatchObject({ title: "Couldn't sync", detail: 'The Deck went offline' });
    vi.advanceTimersByTime(10 * 60_000);
    expect(shown()).toHaveLength(1);
  });

  it('holds a toast while it is pointed at, from where it was', () => {
    const id = toasts.show('Snapshot taken', 'success');
    vi.advanceTimersByTime(TOAST_LIFE - 1000);
    toasts.pause(id);
    vi.advanceTimersByTime(60_000);
    expect(shown()).toHaveLength(1);
    expect(shown()[0]).toMatchObject({ held: true, left: 1000 });
    toasts.resume(id);
    vi.advanceTimersByTime(999);
    expect(shown()).toHaveLength(1);
    vi.advanceTimersByTime(2);
    expect(shown()).toHaveLength(0);
  });

  it('carries out what waits on a toast when it runs out or is closed, and not when it is removed', () => {
    const expired = vi.fn();
    const closed = vi.fn();
    toasts.show('Untracked', 'info', { ttl: 1000, onExpire: expired, onClose: closed });
    vi.advanceTimersByTime(1001);
    expect(expired).toHaveBeenCalledTimes(1);
    expect(closed).not.toHaveBeenCalled();

    const id = toasts.show('Untracked', 'info', { ttl: 1000, onExpire: expired, onClose: closed });
    toasts.close(id);
    expect(closed).toHaveBeenCalledTimes(1);

    const id2 = toasts.show('Untracked', 'info', { ttl: 1000, onExpire: expired, onClose: closed });
    toasts.remove(id2);
    vi.advanceTimersByTime(5000);
    expect(expired).toHaveBeenCalledTimes(1);
    expect(closed).toHaveBeenCalledTimes(1);
  });

  it('updates a toast under way in place, and gives it its full time once done', () => {
    const id = toasts.show('Restoring Hades', 'progress', { key: 'restore:hades', progress: { done: 1, total: 3 } });
    vi.advanceTimersByTime(60_000);
    expect(toasts.show('Restoring Hades', 'progress', { key: 'restore:hades', progress: { done: 2, total: 3 } })).toBe(id);
    expect(shown()).toHaveLength(1);
    expect(shown()[0].progress).toEqual({ done: 2, total: 3 });

    toasts.change(id, { kind: 'success', title: 'Hades restored', detail: '3 files', progress: null });
    expect(shown()[0]).toMatchObject({ id, kind: 'success', title: 'Hades restored', detail: '3 files', progress: null });
    vi.advanceTimersByTime(TOAST_LIFE - 1);
    expect(shown()).toHaveLength(1);
    vi.advanceTimersByTime(2);
    expect(shown()).toHaveLength(0);
  });

  it('keeps holding a toast that changes kind while pointed at', () => {
    const id = toasts.show('Restoring Hades', 'progress', { key: 'r' });
    toasts.pause(id);
    toasts.change(id, { kind: 'success', title: 'Hades restored' });
    vi.advanceTimersByTime(60_000);
    expect(shown()).toHaveLength(1);
    toasts.resume(id);
    vi.advanceTimersByTime(TOAST_LIFE + 1);
    expect(shown()).toHaveLength(0);
  });

  it('never runs out a toast with no end, held or not', () => {
    const id = toasts.show('Restoring', 'progress');
    toasts.pause(id);
    toasts.resume(id);
    vi.advanceTimersByTime(60_000);
    expect(shown()).toHaveLength(1);
  });

  it('lets a toast under way be closed, and shows it again only to say how it ended', () => {
    const id = toasts.show('Restoring Hades', 'progress', { key: 'r', progress: { done: 1, total: 4 } });
    toasts.close(id);
    expect(toasts.show('Restoring Hades', 'progress', { key: 'r', progress: { done: 2, total: 4 } })).toBe(null);
    expect(shown()).toHaveLength(0);
    toasts.show('Hades restored', 'success', { key: 'r' });
    expect(shown()).toMatchObject([{ kind: 'success', title: 'Hades restored' }]);
    // A new restore later is shown again.
    vi.advanceTimersByTime(TOAST_LIFE + 1);
    toasts.show('Restoring Hades', 'progress', { key: 'r' });
    expect(shown()).toHaveLength(1);
  });

  it('counts each time a countdown starts from the top, and not a hold', () => {
    const id = toasts.show('Restoring', 'progress', { key: 'k' });
    expect(shown()[0].round).toBe(0);
    toasts.pause(id);
    toasts.resume(id);
    expect(shown()[0].round).toBe(0);
    toasts.change(id, { kind: 'success', title: 'Done' });
    expect(shown()[0].round).toBe(1);
  });

  it('keeps a changed toast in its place in the list', () => {
    toasts.show('One', 'info');
    const two = toasts.show('Two', 'progress', { key: 'k' });
    toasts.show('Three', 'info');
    toasts.change(two, { kind: 'success', title: 'Two done' });
    expect(shown().map((t) => t.title)).toEqual(['One', 'Two done', 'Three']);
  });
});
