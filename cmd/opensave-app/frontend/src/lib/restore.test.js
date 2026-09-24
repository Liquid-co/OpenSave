import { describe, expect, it } from 'vitest';
import { get } from 'svelte/store';
import { answerRestore, askRestore, describeSummary, restoreRequest, summarize } from './restore.js';

describe('summarize', () => {
  const preview = {
    unchanged: 3,
    changes: [
      { path: 'z.sav', change: 'removed' },
      { path: 'b.sav', change: 'changed' },
      { location: 'config', path: 'settings.ini', change: 'changed' },
      { path: 'a.sav', change: 'restored' },
      { path: 'a.sav', change: 'changed' }
    ]
  };

  it('counts each kind and orders by location, then what happens, then path', () => {
    const s = summarize(preview);
    expect([s.changed, s.restored, s.removed, s.unchanged]).toEqual([3, 1, 1, 3]);
    expect(s.changes.map((c) => `${c.location ?? ''}:${c.change}:${c.path}`)).toEqual([
      ':changed:a.sav',
      ':changed:b.sav',
      ':restored:a.sav',
      ':removed:z.sav',
      'config:changed:settings.ini'
    ]);
    expect(s.identical).toBe(false);
  });

  it('knows when a restore would change nothing', () => {
    expect(summarize({ changes: [], unchanged: 4 }).identical).toBe(true);
    expect(summarize(null).identical).toBe(true);
  });
});

describe('describeSummary', () => {
  it('says only what applies, in the singular or plural', () => {
    expect(describeSummary({ changed: 2, restored: 1, removed: 1 })).toBe('2 files change, 1 comes back, 1 is removed');
    expect(describeSummary({ changed: 1, restored: 0, removed: 3 })).toBe('1 file changes, 3 are removed');
    expect(describeSummary({ changed: 0, restored: 2, removed: 0 })).toBe('2 come back');
  });
});

describe('askRestore', () => {
  it('resolves with the answer and closes', async () => {
    const pending = askRestore({ id: 'g' }, { id: 's' });
    expect(get(restoreRequest).snap.id).toBe('s');
    answerRestore(true);
    expect(await pending).toBe(true);
    expect(get(restoreRequest)).toBe(null);
  });
});
