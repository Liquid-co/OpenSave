// Getting set up: find your saves, add your other devices, back up to the
// cloud. Each step is done when the thing it asks for exists — not when a
// button in the guide was pressed — so doing it anywhere else in the app
// ticks it off too, and nothing is claimed that isn't so.
//
// A step can be skipped (one device, no cloud: both reasonable), and the
// guide as a whole can be put away. Remembered on this device.
import { writable } from 'svelte/store';

export const SETUP_STEPS = ['saves', 'devices', 'cloud'];

/** Each step with whether it is done or skipped, in order, and the one to do next. */
export function setupSteps({ games = 0, peers = 0, cloud = false, skipped = [] } = {}) {
  const done = { saves: games > 0, devices: peers > 0, cloud: !!cloud };
  const steps = SETUP_STEPS.map((id) => ({ id, done: done[id], skipped: !done[id] && skipped.includes(id) }));
  const next = steps.find((s) => !s.done && !s.skipped)?.id ?? null;
  const settled = steps.filter((s) => s.done || s.skipped).length;
  return { steps, next, settled, finished: next === null };
}

const KEY = 'opensave.setup';

// `seen` is whether this device has decided yet whether the guide is for it.
// It is decided once, the first time the library loads: a library that
// already has games belongs to someone who set up before the guide existed,
// and an update should not greet them with a first-run checklist.
export function loadSetup(storage = globalThis.localStorage) {
  try {
    const v = JSON.parse(storage?.getItem(KEY) ?? 'null');
    return {
      seen: !!v?.seen,
      dismissed: !!v?.dismissed,
      skipped: Array.isArray(v?.skipped) ? v.skipped.filter((s) => SETUP_STEPS.includes(s)) : []
    };
  } catch {
    return { seen: false, dismissed: false, skipped: [] };
  }
}

export function saveSetup(v, storage = globalThis.localStorage) {
  try {
    storage?.setItem(KEY, JSON.stringify({ seen: !!v.seen, dismissed: !!v.dismissed, skipped: v.skipped ?? [] }));
  } catch {
    // Kept until restart.
  }
}

/** The first time the library is known: the guide is for a library that
 *  starts empty. Later calls change nothing. */
export function decideSetupFor(state, gameCount) {
  if (state.seen) return state;
  return { ...state, seen: true, dismissed: state.dismissed || gameCount > 0 };
}

export const setupState = writable(loadSetup());
setupState.subscribe((v) => saveSetup(v));

/** Brings the guide back on Home — from Settings or Ctrl+K — with any
 *  skipped step offered again. Steps already done stay ticked, since they
 *  are worked out from what exists. */
export function showSetupAgain() {
  setupState.update((s) => ({ ...s, seen: true, dismissed: false, skipped: [] }));
}
