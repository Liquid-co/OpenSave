// Pairing requests this device sent, waiting on the other device to accept.
//
// Kept here rather than in whichever part sent them, so the Devices page and
// the Add a device dialog show the same request with the same numbers.
//
// A request to a device nearby comes back with four digits (the daemon's
// pairing/check.go). The other device shows the same four only if nothing in
// between swapped the keys, so the user accepts there only when they match.
// A device on a version without the check gives none.
import { readable, writable } from 'svelte/store';
import { api } from './api.js';

/** How long the other device has to accept: the daemon's grace window. */
export const ASK_WINDOW_MS = 2 * 60 * 1000;

/** key → { name, code, at }. key is the device id, or ip:port when typed in. */
export const sentPairings = writable({});

/** Ticks so a request that has run out stops showing as waiting. */
export const pairClock = readable(Date.now(), (set) => {
  const id = setInterval(() => set(Date.now()), 5000);
  return () => clearInterval(id);
});

/** Ask a device to pair, and remember the request under key. */
export async function askToPair(key, name, body, now = Date.now()) {
  const res = await api.post('/api/peers/pair', body);
  sentPairings.update((m) => ({ ...m, [key]: { name, code: checkDigits(res?.checkCode).join(''), at: now } }));
  return res;
}

/** The request sent under key, while the other device can still accept it. */
export function waitingOn(map, key, now = Date.now()) {
  const s = map?.[key];
  return s && now - s.at < ASK_WINDOW_MS ? s : null;
}

/** The four digits of a check code, or [] for anything that is not one. */
export function checkDigits(code) {
  const s = String(code ?? '');
  return /^\d{4}$/.test(s) ? s.split('') : [];
}
