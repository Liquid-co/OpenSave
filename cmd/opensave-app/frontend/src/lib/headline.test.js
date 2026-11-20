import { describe, expect, it } from 'vitest';
import { libraryHeadline } from './headline.js';

const snap = { id: 's', timestamp: '2026-11-20T10:00:00Z' };
const game = (name, extra = {}) => ({ id: name, name, activeBranch: 'main', branches: { main: { snapshots: [snap] } }, ...extra });
const row = (name, state, extra) => ({ game: game(name, extra), status: { state } });

describe('libraryHeadline', () => {
  it('says everything is backed up when nothing needs saying', () => {
    expect(libraryHeadline([row('A', 'synced'), row('B', 'synced')])).toEqual({ key: 'home.summaryLine.backedUpMany', params: { count: 2 } });
    expect(libraryHeadline([row('A', 'synced')])).toEqual({ key: 'home.summaryLine.backedUpOne', params: {} });
  });

  it('puts a missing folder ahead of everything else, by name when there is one', () => {
    const rows = [row('A', 'conflict'), row('B', 'missing'), row('C', 'syncing')];
    expect(libraryHeadline(rows)).toEqual({ key: 'home.summaryLine.missingOne', params: { name: 'B' } });
  });

  it('counts when there are several', () => {
    expect(libraryHeadline([row('A', 'conflict'), row('B', 'conflict')])).toEqual({ key: 'home.summaryLine.conflictMany', params: { count: 2 } });
  });

  it('finds a damaged snapshot whatever the state says', () => {
    const rows = [row('A', 'synced', { branches: { main: { snapshots: [{ ...snap, problem: 'unreadable' }] } } })];
    expect(libraryHeadline(rows)).toEqual({ key: 'home.summaryLine.damagedOne', params: { name: 'A' } });
  });

  it('notices games with no snapshot yet', () => {
    const rows = [row('A', 'synced', { branches: { main: { snapshots: [] } } }), row('B', 'synced')];
    expect(libraryHeadline(rows).key).toBe('home.summaryLine.emptyOne');
  });
});
