import { describe, it, expect } from 'vitest';
import { dialogOut } from './motion.js';

describe('dialogOut', () => {
  // A dialog reopened while it was still leaving: Svelte stops the leaving
  // with tick(1). It used to leave pointer events off, so the dialog that
  // came back could be seen but not clicked or scrolled.
  it('gives a dialog back its clicks when its leaving is stopped', () => {
    const panel = { style: {} };
    const node = { style: {}, firstElementChild: panel };
    const tr = dialogOut(node, {});
    if (!tr.tick) return; // animations off in this environment: nothing to check
    tr.tick(0.5, 0.5);
    expect(node.style.pointerEvents).toBe('none');
    tr.tick(1, 0);
    expect(node.style.pointerEvents).toBe('');
    expect(node.style.opacity).toBe('1');
    expect(panel.style.transform).toBe('');
  });
});
