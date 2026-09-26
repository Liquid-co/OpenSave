// Attention helpers shared by the pairing banner and conflict modal, and the
// desktop notifications every kind of interruption can come with.
import { get } from 'svelte/store';
import { native, gameCover } from './api.js';
import { mayInterrupt, notifyPrefs } from './notifyprefs.js';

/** A short, pleasant two-note chime synthesised on the fly (no asset). */
export function playChime() {
  try {
    const Ctx = window.AudioContext || window.webkitAudioContext;
    if (!Ctx) return;
    const ctx = new Ctx();
    if (ctx.state === 'suspended') ctx.resume();
    const now = ctx.currentTime;
    [660, 990].forEach((freq, i) => {
      const osc = ctx.createOscillator();
      const gain = ctx.createGain();
      osc.type = 'sine';
      osc.frequency.value = freq;
      osc.connect(gain);
      gain.connect(ctx.destination);
      const t = now + i * 0.13;
      gain.gain.setValueAtTime(0, t);
      gain.gain.linearRampToValueAtTime(0.18, t + 0.02);
      gain.gain.exponentialRampToValueAtTime(0.0001, t + 0.35);
      osc.start(t);
      osc.stop(t + 0.36);
    });
    setTimeout(() => ctx.close(), 900);
  } catch {}
}

/** Chime + surface the window — for events that need the user's eyes, when
 *  that event may interrupt now (see notifyprefs.js). `event` names it;
 *  `note`, when given, is also shown on the desktop (toDesktop). */
export async function demandAttention(event, note) {
  // Asked before the window comes up, which makes it the one in front.
  const front = inFront();
  if (!(await mayInterrupt(event, native.userBusy))) return;
  playChime();
  if (note) toDesktop(event, note, { front });
  native.showWindow();
}

/** Whether OpenSave is in front: its window showing and focused. */
export function inFront(doc = globalThis.document) {
  return !!doc && doc.visibilityState === 'visible' && typeof doc.hasFocus === 'function' && doc.hasFocus();
}

/**
 * What to show on the desktop for an event, or null for nothing. Only when
 * OpenSave is not in front to show it itself, and only for an event left on,
 * with desktop notifications on. `note` is {title, body, game?, image?,
 * open?}: a game's shows its cover and opens its page when clicked; `open`,
 * a view and its params, sends a click elsewhere.
 */
export function desktopNote(event, note, prefs, front) {
  if (!prefs?.desktop || !prefs[event] || front || !note?.title) return null;
  const game = note.game;
  return {
    title: note.title,
    body: note.body ?? '',
    image: note.image ?? (game ? gameCover(game) : ''),
    open: JSON.stringify(note.open ?? (game ? { view: 'game', params: { gameId: game.id } } : { view: 'home', params: {} }))
  };
}

/** Shows an event on the desktop when desktopNote says to — and, with quiet
 *  while playing on, not over a full-screen game. Resolves to whether it did. */
export async function toDesktop(event, note, { prefs = get(notifyPrefs), front = inFront(), busy = native.userBusy, send = native.desktopNotify } = {}) {
  const n = desktopNote(event, note, prefs, front);
  if (!n) return false;
  if (prefs.quietWhilePlaying) {
    try {
      if (await busy()) return false;
    } catch {
      // Not knowing is not a reason to stay silent.
    }
  }
  try {
    return !(await send(n));
  } catch {
    return false;
  }
}
