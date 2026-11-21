import { describe, expect, it } from 'vitest';
import { get } from 'svelte/store';
import { createRunner } from './runner.js';

const notes = () => {
  const seen = [];
  return { seen, notify: (msg, kind, opts) => seen.push([kind, msg, opts]) };
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
    expect(seen.map(([k, m]) => [k, m])).toEqual([['success', 'Done']]);
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
    expect(seen.map(([k, m]) => [k, m])).toEqual([['error', 'the daemon said no']]);
    expect(get(busy)).toBe(false);
  });

  it('says whether the action ran and succeeded', async () => {
    const { run } = createRunner(() => {});
    expect(await run('', async () => {})).toBe(true);
    expect(
      await run('', async () => {
        throw new Error('no');
      })
    ).toBe(false);
  });

  it('says nothing on success without a label', async () => {
    const { seen, notify } = notes();
    await createRunner(notify).run('', async () => {});
    expect(seen).toEqual([]);
  });
});

describe('a failed action', () => {
  it('offers to try it again, which runs it again', async () => {
    const { seen, notify } = notes();
    const { run } = createRunner(notify);
    let calls = 0;
    const fn = async () => {
      calls++;
      if (calls === 1) throw new Error('daemon said no');
    };
    expect(await run('Done', fn, { key: 'k', action: { label: 'Open' } })).toBe(false);
    const [kind, msg, opts] = seen[0];
    expect([kind, msg, opts.key, opts.action]).toEqual(['error', 'daemon said no', 'k', undefined]);
    await opts.retry();
    expect(calls).toBe(2);
    expect(seen[1].slice(0, 2)).toEqual(['success', 'Done']);
    expect(seen[1][2].action).toEqual({ label: 'Open' });
  });
});
