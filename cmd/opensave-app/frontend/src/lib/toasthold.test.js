import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { createHold } from './toasthold.js';

describe('createHold', () => {
  let changes;
  let hold;
  beforeEach(() => {
    vi.useFakeTimers();
    changes = [];
    hold = createHold({ onChange: (h) => changes.push(h), setTimer: setTimeout, clearTimer: clearTimeout });
  });
  afterEach(() => vi.useRealTimers());

  it('holds while the pointer is on the toasts, and lets go a moment after it leaves', () => {
    hold.pointerIn();
    expect(changes).toEqual([true]);
    hold.pointerOut();
    vi.advanceTimersByTime(249);
    expect(hold.holding).toBe(true);
    vi.advanceTimersByTime(2);
    expect(changes).toEqual([true, false]);
  });

  it('keeps holding when the pointer crosses a gap and comes back', () => {
    hold.pointerIn();
    hold.pointerOut();
    vi.advanceTimersByTime(100);
    hold.pointerIn();
    vi.advanceTimersByTime(1000);
    expect(changes).toEqual([true]);
  });

  it('lets go when moves keep coming from outside, not only on the first', () => {
    hold.pointerIn();
    for (let i = 0; i < 10; i++) {
      hold.pointerOut();
      vi.advanceTimersByTime(50);
    }
    expect(changes).toEqual([true, false]);
  });

  it('holds while focus is in a toast, whatever the pointer does', () => {
    hold.focusIn();
    hold.pointerIn();
    hold.pointerOut();
    vi.advanceTimersByTime(1000);
    expect(hold.holding).toBe(true);
    hold.focusOut();
    expect(changes).toEqual([true, false]);
  });

  it('holds a stack tapped open until it is tapped shut', () => {
    hold.pin(true);
    vi.advanceTimersByTime(10_000);
    expect(hold.holding).toBe(true);
    hold.pin(false);
    expect(changes).toEqual([true, false]);
  });

  it('ignores a pointer leaving that never came in', () => {
    hold.pointerOut();
    vi.advanceTimersByTime(1000);
    expect(changes).toEqual([]);
  });
});
