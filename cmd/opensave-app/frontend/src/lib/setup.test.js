import { describe, expect, it } from 'vitest';
import { decideSetupFor, loadSetup, saveSetup, setupSteps } from './setup.js';

describe('setupSteps', () => {
  it('starts at finding saves, with nothing done', () => {
    const s = setupSteps({});
    expect(s.next).toBe('saves');
    expect(s.settled).toBe(0);
    expect(s.finished).toBe(false);
  });

  it('counts a step done by what exists, however it came to exist', () => {
    const s = setupSteps({ games: 3, peers: 1, cloud: false });
    expect(s.steps.map((x) => x.done)).toEqual([true, true, false]);
    expect(s.next).toBe('cloud');
  });

  it('moves past a skipped step, and a skipped step done later counts as done', () => {
    const s = setupSteps({ games: 2, skipped: ['devices'] });
    expect(s.next).toBe('cloud');
    expect(s.steps[1]).toMatchObject({ done: false, skipped: true });
    const later = setupSteps({ games: 2, peers: 1, skipped: ['devices'] });
    expect(later.steps[1]).toMatchObject({ done: true, skipped: false });
  });

  it('is finished when every step is done or skipped', () => {
    expect(setupSteps({ games: 1, skipped: ['devices', 'cloud'] }).finished).toBe(true);
    expect(setupSteps({ games: 0, skipped: ['devices', 'cloud'] }).finished).toBe(false);
  });
});

describe('loadSetup and saveSetup', () => {
  it('round-trips, ignores unknown steps, and survives garbage', () => {
    const data = {};
    const s = { getItem: (k) => data[k] ?? null, setItem: (k, v) => (data[k] = v) };
    saveSetup({ seen: true, dismissed: true, skipped: ['cloud'] }, s);
    expect(loadSetup(s)).toEqual({ seen: true, dismissed: true, skipped: ['cloud'] });
    data['opensave.setup'] = JSON.stringify({ skipped: ['cloud', 'bogus'] });
    expect(loadSetup(s).skipped).toEqual(['cloud']);
    data['opensave.setup'] = '{nope';
    expect(loadSetup(s)).toEqual({ seen: false, dismissed: false, skipped: [] });
  });
});

describe('decideSetupFor', () => {
  const fresh = { seen: false, dismissed: false, skipped: [] };
  it('offers the guide to a library that starts empty', () => {
    expect(decideSetupFor(fresh, 0)).toEqual({ seen: true, dismissed: false, skipped: [] });
  });
  it('does not greet someone who set up before the guide existed', () => {
    expect(decideSetupFor(fresh, 5).dismissed).toBe(true);
  });
  it('decides once: games tracked later in the first run do not put it away', () => {
    const decided = decideSetupFor(fresh, 0);
    expect(decideSetupFor(decided, 3)).toBe(decided);
  });
});

describe('showing the guide again', () => {
  it('brings it back with skipped steps offered again', async () => {
    const { setupState, showSetupAgain } = await import('./setup.js');
    const { get } = await import('svelte/store');
    setupState.set({ seen: true, dismissed: true, skipped: ['cloud'] });
    showSetupAgain();
    expect(get(setupState)).toEqual({ seen: true, dismissed: false, skipped: [] });
  });
});
