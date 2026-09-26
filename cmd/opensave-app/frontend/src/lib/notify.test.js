import { describe, expect, it } from 'vitest';
import { desktopNote, toDesktop, inFront } from './notify.js';
import { arrivalNote, newGamesNote } from './notifications.js';
import { DEFAULT_NOTIFY, sanitizeNotify } from './notifyprefs.js';

const hades = { id: 'hades', name: 'Hades', appId: '1145360', savePath: '/x/Hades' };
const prefs = { ...DEFAULT_NOTIFY };

describe('desktop notifications', () => {
  it('are on unless turned off, and kept when saved', () => {
    expect(DEFAULT_NOTIFY.desktop).toBe(true);
    expect(sanitizeNotify({ desktop: false }).desktop).toBe(false);
    expect(sanitizeNotify({}).desktop).toBe(true);
  });

  it('show only when OpenSave is not in front, for an event left on', () => {
    const note = { title: 'Hades', body: 'Got 2 files from Steam Deck', game: hades };
    expect(desktopNote('arrivals', note, prefs, true)).toBeNull();
    expect(desktopNote('arrivals', note, { ...prefs, arrivals: false }, false)).toBeNull();
    expect(desktopNote('arrivals', note, { ...prefs, desktop: false }, false)).toBeNull();
    expect(desktopNote('arrivals', null, prefs, false)).toBeNull();

    const n = desktopNote('arrivals', note, prefs, false);
    expect(n).toMatchObject({ title: 'Hades', body: 'Got 2 files from Steam Deck' });
    expect(n.image).toContain('appId=1145360');
    expect(JSON.parse(n.open)).toEqual({ view: 'game', params: { gameId: 'hades' } });

    const pairing = desktopNote('pairing', { title: 'Steam Deck wants to pair', open: { view: 'devices', params: {} } }, prefs, false);
    expect(pairing.image).toBe('');
    expect(JSON.parse(pairing.open).view).toBe('devices');
  });

  it('are held back over a full-screen game when quiet while playing is on', async () => {
    const sent = [];
    const send = async (n) => {
      sent.push(n);
      return '';
    };
    const note = { title: 'Hades', body: 'x', game: hades };
    expect(await toDesktop('arrivals', note, { prefs, front: false, busy: async () => true, send })).toBe(false);
    expect(await toDesktop('arrivals', note, { prefs: { ...prefs, quietWhilePlaying: false }, front: false, busy: async () => true, send })).toBe(true);
    expect(await toDesktop('arrivals', note, { prefs, front: false, busy: async () => false, send })).toBe(true);
    expect(await toDesktop('arrivals', note, { prefs, front: false, busy: async () => false, send: async () => 'no' })).toBe(false);
    expect(sent).toHaveLength(2);
  });

  it('say what happened plainly', () => {
    expect(arrivalNote({ gameId: 'hades', files: 3, device: 'Steam Deck' }, { hades })).toMatchObject({
      title: 'Hades',
      body: 'Got 3 files from Steam Deck',
      game: hades
    });
    expect(arrivalNote({ gameId: 'gone' }, { hades })).toBeNull();
    expect(newGamesNote([{ name: 'Balatro' }]).title).toBe('New game found');
    expect(newGamesNote([{ name: 'A' }, { name: 'B' }, { name: 'C' }, { name: 'D' }]).body).toMatch(/^A, B and 2 more/);
    expect(newGamesNote([])).toBeNull();
  });

  it('know whether OpenSave is in front', () => {
    expect(inFront({ visibilityState: 'visible', hasFocus: () => true })).toBe(true);
    expect(inFront({ visibilityState: 'hidden', hasFocus: () => true })).toBe(false);
    expect(inFront({ visibilityState: 'visible', hasFocus: () => false })).toBe(false);
    expect(inFront(undefined)).toBe(false);
  });
});
