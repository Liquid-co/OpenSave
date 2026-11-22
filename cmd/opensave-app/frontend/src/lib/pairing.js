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

/**
 * What is wrong with an address typed in to pair with, or '' when it will do:
 * 'address' for something that is no host or IP address (spaces, stray
 * characters), 'port' for a port outside 1-65535. Not whether anything
 * answers there: that only the request can tell.
 */
export function addressProblem(ip, port) {
  const host = String(ip ?? '').trim();
  if (!host) return '';
  // An IPv4 address or a name (letters, digits, dots, hyphens), or an IPv6
  // address, bare or in brackets.
  const name = /^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?)*$/;
  const v6 = /^\[?[0-9A-Fa-f:.]*:[0-9A-Fa-f:.]*\]?$/;
  const v4 = /^\d{1,3}(\.\d{1,3}){3}$/;
  if (v4.test(host) && host.split('.').some((n) => Number(n) > 255)) return 'address';
  const isV6 = v6.test(host) && host.split(':').length >= 3;
  if (!name.test(host) && !isV6) return 'address';
  const p = Number(port);
  if (!Number.isInteger(p) || p < 1 || p > 65535) return 'port';
  return '';
}
