import { describe, expect, it } from 'vitest';
import { dayLabel, groupByDay, humanizeComment, snapIdTime, snapshotKind, whenLabel } from './snapshots.js';

// Local time throughout: these format for the person looking at them.
const now = new Date(2026, 8, 24, 16, 0, 0);
const at = (d, h, m = 0) => new Date(2026, 8, d, h, m, 0);

describe('snapIdTime', () => {
  it('reads the time out of an ID', () => {
    const t = at(24, 13, 5);
    expect(snapIdTime(`snap_${t.getTime()}`).getTime()).toBe(t.getTime());
  });
  it('is null for anything else', () => {
    expect(snapIdTime('current')).toBe(null);
    expect(snapIdTime('snap_12')).toBe(null);
    expect(snapIdTime(undefined)).toBe(null);
  });
});

describe('whenLabel and dayLabel', () => {
  it('gives only the time for today, and the date too otherwise', () => {
    expect(whenLabel(at(24, 9, 30), now)).toBe(at(24, 9, 30).toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' }));
    expect(whenLabel(at(20, 9, 30), now)).toContain(at(20, 9, 30).toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' }));
    expect(whenLabel(at(20, 9, 30), now)).not.toBe(whenLabel(at(24, 9, 30), now));
  });
  it('names today and yesterday, and dates the rest', () => {
    expect(dayLabel(at(24, 1), now)).toBe('Today');
    expect(dayLabel(at(23, 23), now)).toBe('Yesterday');
    expect(dayLabel(at(20, 12), now)).not.toMatch(/Today|Yesterday/);
  });
});

describe('humanizeComment', () => {
  it('replaces snapshot IDs with the time they name', () => {
    const id = `snap_${at(24, 13, 5).getTime()}`;
    const out = humanizeComment(`Pre-rollback safety restore point (before restoring ${id})`, now);
    expect(out).not.toContain('snap_');
    expect(out).toContain(`the ${whenLabel(at(24, 13, 5), now)} snapshot`);
  });
  it('leaves other text alone', () => {
    expect(humanizeComment('before the boss', now)).toBe('before the boss');
    expect(humanizeComment(undefined, now)).toBe('');
  });
});

describe('snapshotKind', () => {
  const auto = (comment) => ({ isSystemAuto: true, comment });

  it('calls one the person took theirs, titled by their comment', () => {
    expect(snapshotKind({ isSystemAuto: false, comment: 'before the boss' })).toMatchObject({
      kind: 'manual',
      title: 'before the boss'
    });
    expect(snapshotKind({ isSystemAuto: false, comment: '' }).title).toBe('Snapshot');
    // What the daemon stores when no comment was given.
    expect(snapshotKind({ isSystemAuto: false, comment: 'Manual snapshot' }).title).toBe('Snapshot');
  });

  it('marks the save as a play session left it', () => {
    expect(snapshotKind(auto('After playing (1 h 12 min)'))).toMatchObject({
      kind: 'session',
      label: 'After playing',
      title: 'After playing (1 h 12 min)'
    });
  });

  it('says a save changed for the watcher’s own snapshots', () => {
    expect(snapshotKind(auto(''))).toMatchObject({ kind: 'auto', title: 'Save changed' });
    expect(snapshotKind(auto('Auto backup'))).toMatchObject({ kind: 'auto', title: 'Save changed' });
  });

  it('recognises the first snapshot', () => {
    expect(snapshotKind(auto('Initial snapshot')).kind).toBe('start');
  });

  it('marks every copy kept before the save was replaced as a safety copy', () => {
    for (const c of [
      'before sync replaced local files',
      "This device's version (before keeping Deck's)",
      'Pre-rollback safety restore point (before restoring snap_1790235804433)',
      'Safety snapshot before restoring file "a.sav" from snap_1790235804433',
      'Auto backup before switching to branch "ng"',
      'Before resolving the "config" save location with Deck'
    ]) {
      const k = snapshotKind(auto(c), now);
      expect(k.kind, c).toBe('safety');
      expect(k.title[0]).toBe(k.title[0].toUpperCase());
      expect(k.title).not.toContain('snap_');
    }
  });

  it('says the daemon’s own reasons plainly', () => {
    const id = `snap_${at(24, 13, 5).getTime()}`;
    const t = whenLabel(at(24, 13, 5), now);
    const title = (c) => snapshotKind(auto(c), now).title;
    expect(title(`Pre-rollback safety restore point (before restoring ${id})`)).toBe(`Before restoring the ${t} snapshot`);
    expect(title('before sync replaced local files')).toBe('Before a sync replaced it');
    expect(title(`Safety snapshot before restoring file "a.sav" from ${id}`)).toBe(`Before putting back a.sav from the ${t} snapshot`);
    expect(title('Auto backup before switching to branch "ng"')).toBe('Before switching to ng');
    expect(title('Branch "ng" created from "main"')).toBe('Start of ng, copied from main');
  });

  it('keeps any other automatic reason as it was written', () => {
    expect(snapshotKind(auto('Imported from backup'))).toMatchObject({ kind: 'other', title: 'Imported from backup' });
  });
});

describe('groupByDay', () => {
  it('runs consecutive snapshots of one day together, newest first', () => {
    const snaps = [
      { id: 'a', timestamp: at(24, 15).toISOString() },
      { id: 'b', timestamp: at(24, 9).toISOString() },
      { id: 'c', timestamp: at(23, 20).toISOString() },
      { id: 'd', timestamp: at(20, 8).toISOString() }
    ];
    const g = groupByDay(snaps, now);
    expect(g.map((x) => [x.day === 'Today' || x.day === 'Yesterday' ? x.day : 'date', x.snaps.map((s) => s.id)])).toEqual([
      ['Today', ['a', 'b']],
      ['Yesterday', ['c']],
      ['date', ['d']]
    ]);
  });
});
