import { tr } from './i18n.js';
// Which events interrupt you, on this device.
//
// Two kinds of interruption: a chime with the window brought to the front,
// for the things that are waiting on you (a device asking to pair, two saves
// in conflict); and a message in the corner, for things that happened (a
// newer save brought from the cloud, a new game found). Each can be turned
// off; what it was about is still on screen when you look.
//
// Either also shows on the desktop, as the system's own notification, when
// OpenSave is not in front to show it (lib/notify.js) — unless "desktop" is
// turned off.
//
// "Quiet while playing" holds the chime, the window and the desktop
// notification back while a full-screen game is running, where all three are
// at their most unwelcome — the window jumping in front of a game can take
// focus out of it. The banner or dialog is there when you come back.
import { writable, get } from 'svelte/store';

export const NOTIFY_EVENTS = {
  pairing: { get label() { return tr('notifyprefs.aDeviceAsksToPair'); }, kind: 'attention' },
  conflicts: { get label() { return tr('notifyprefs.twoSavesConflict'); }, kind: 'attention' },
  emptied: { get label() { return tr('notifyprefs.everySaveFileOfAGameIs'); }, kind: 'attention' },
  arrivals: { get label() { return tr('notifyprefs.aSaveArrivesFromAnotherDevice'); }, kind: 'message' },
  cloudPulled: { get label() { return tr('notifyprefs.aNewerSaveIsBroughtFromThe'); }, kind: 'message' },
  newGames: { get label() { return tr('notifyprefs.aNewlyInstalledGameIsFound'); }, kind: 'message' }
};

export const DEFAULT_NOTIFY = Object.freeze({
  pairing: true,
  conflicts: true,
  emptied: true,
  arrivals: true,
  cloudPulled: true,
  newGames: true,
  desktop: true,
  quietWhilePlaying: true
});

export function sanitizeNotify(raw) {
  const v = raw && typeof raw === 'object' ? raw : {};
  const out = {};
  for (const k of Object.keys(DEFAULT_NOTIFY)) out[k] = typeof v[k] === 'boolean' ? v[k] : DEFAULT_NOTIFY[k];
  return out;
}

const KEY = 'opensave.notifications';

export function loadNotify(storage = globalThis.localStorage) {
  try {
    const saved = storage?.getItem(KEY);
    return saved ? sanitizeNotify(JSON.parse(saved)) : { ...DEFAULT_NOTIFY };
  } catch {
    return { ...DEFAULT_NOTIFY };
  }
}

export function saveNotify(v, storage = globalThis.localStorage) {
  try {
    storage?.setItem(KEY, JSON.stringify(sanitizeNotify(v)));
  } catch {
    // Kept until restart.
  }
}

export const notifyPrefs = writable(loadNotify());
notifyPrefs.subscribe((v) => saveNotify(v));

/**
 * Whether an event may interrupt now: turned on, and — for a chime and a
 * window in front — not while a full-screen game is running when quiet is on.
 * `busy` answers that last question (the app asks the operating system).
 */
export async function mayInterrupt(event, busy, prefs = get(notifyPrefs)) {
  if (!prefs[event]) return false;
  if (NOTIFY_EVENTS[event]?.kind === 'attention' && prefs.quietWhilePlaying) {
    try {
      if (await busy()) return false;
    } catch {
      // Not knowing is not a reason to stay silent.
    }
  }
  return true;
}
