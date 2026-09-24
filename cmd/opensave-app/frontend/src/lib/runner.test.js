import { describe, expect, it } from 'vitest';
import { get } from 'svelte/store';
import { createRunner } from './runner.js';

const notes = () => {
  const seen = [];
  return { seen, notify: (msg, kind) => seen.push([kind, msg]) };
};

describe('createRunner', () => {
  it('is busy while an action runs, and announces success', async () => {
    const { seen, notify } = notes();
    const { busy, run } = createRunner(notify);
    let release;
    const p = run('Done', () => new Promise((r) => (release = r)));
    expect(get(busy)).toBe(true);
    release();
    await p;
    expect(get(busy)).toBe(false);
    expect(seen).toEqual([['success', 'Done']]);
  });

  it('refuses a second action while the first is running', async () => {
    const { notify } = notes();
    const { run } = createRunner(notify);
    let release;
    let second = false;
    const p = run('', () => new Promise((r) => (release = r)));
    await run('', async () => (second = true));
    expect(second).toBe(false);
    release();
    await p;
    await run('', async () => (second = true));
    expect(second).toBe(true);
  });

  it('announces a failure with its message and frees the screen again', async () => {
    const { seen, notify } = notes();
    const { busy, run } = createRunner(notify);
    await run('Never said', async () => {
      throw new Error('the daemon said no');
    });
    expect(seen).toEqual([['error', 'the daemon said no']]);
    expect(get(busy)).toBe(false);
  });

  it('says nothing on success without a label', async () => {
    const { seen, notify } = notes();
    await createRunner(notify).run('', async () => {});
    expect(seen).toEqual([]);
  });
});
