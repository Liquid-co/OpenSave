// Using OpenSave from a controller — on a Steam Deck or a handheld, or a pad
// plugged into a PC — and from the arrow keys the same way.
//
// Two ways in, one behaviour. The Gamepad API reads a controller directly;
// and on a Steam Deck in desktop mode Steam Input already turns the D-pad into
// arrow keys and A and B into Enter and Escape, so the arrow keys have to do
// the same thing. Either way focus moves to the nearest control in the
// direction pressed (lib/spatial.js), A presses it, B backs out.
//
// Off for a mouse-and-keyboard desktop, where the arrow keys scroll: it turns
// itself on the first time a controller is used, on a device set up as a
// Steam Deck or a handheld, or when Settings says so.
import { get, writable } from 'svelte/store';
import { LAYERS, moveFocus } from './spatial.js';
import { isTyping, PAGES } from './shortcuts.js';

export const CONTROLLER_MODES = { auto: 'Automatic', on: 'On', off: 'Off' };

/** Whether a controller has been used since the app opened. */
export const padUsed = writable(false);

/** Whether controller navigation is on, for this mode and device. */
export function controllerOn(mode, { deviceType = 'desktop', used = false } = {}) {
  if (mode === 'on') return true;
  if (mode === 'off') return false;
  return used || deviceType === 'deck' || deviceType === 'handheld';
}

// Standard gamepad mapping.
const PAD = { a: 0, b: 1, x: 2, y: 3, lb: 4, rb: 5, back: 8, start: 9, up: 12, down: 13, left: 14, right: 15 };
const REPEAT_FIRST = 380;
const REPEAT_EVERY = 110;
const STICK = 0.6;

/**
 * Starts listening. `isOn()` says whether navigation is on right now;
 * `actions` carries what the buttons do that is not moving focus:
 * {back, menu, palette, page(delta)}. Returns a function that stops it.
 */
export function startController({ isOn, actions, win = globalThis.window, doc = globalThis.document }) {
  if (!win) return () => {};

  function onKey(e) {
    if (!isOn() || e.defaultPrevented || e.altKey || e.ctrlKey || e.metaKey) return;
    const dir = { ArrowUp: 'up', ArrowDown: 'down', ArrowLeft: 'left', ArrowRight: 'right' }[e.key];
    if (!dir || isTyping(e.target)) return;
    if (regain() || moveFocus(dir, doc)) e.preventDefault();
  }
  win.addEventListener('keydown', onKey);

  // The last control in focus in the page itself. A dialog or menu that
  // closes can leave nothing in focus; the next press puts focus back there,
  // where it was, rather than starting again at the top of the page.
  let last = null;
  const onFocusIn = (e) => {
    if (!e.target?.closest?.(LAYERS)) last = e.target;
  };
  doc.addEventListener('focusin', onFocusIn);
  function regain() {
    const active = doc.activeElement;
    if (active && active !== doc.body) return false;
    if (!last?.isConnected || doc.querySelector(LAYERS)) return false;
    last.focus();
    last.scrollIntoView?.({ block: 'nearest' });
    return true;
  }

  // The pad is polled while one is connected: the Gamepad API has no events
  // for buttons, only for arrival and departure.
  let frame = 0;
  const held = new Map(); // button -> {since, last}
  function pressed(pad, b) {
    return !!pad.buttons[b]?.pressed;
  }
  function stickDir(pad) {
    const [x = 0, y = 0] = pad.axes;
    if (Math.abs(x) < STICK && Math.abs(y) < STICK) return null;
    return Math.abs(x) > Math.abs(y) ? (x > 0 ? 'right' : 'left') : y > 0 ? 'down' : 'up';
  }
  function fire(button, now, repeat, act) {
    const h = held.get(button);
    if (!h) {
      held.set(button, { since: now, last: now });
      act();
    } else if (repeat && now - h.since > REPEAT_FIRST && now - h.last > REPEAT_EVERY) {
      h.last = now;
      act();
    }
  }
  function poll(now) {
    frame = win.requestAnimationFrame(poll);
    const pads = win.navigator.getGamepads?.() ?? [];
    const pad = [...pads].find((p) => p && p.connected);
    if (!pad) return;
    const down = new Set();
    const stick = stickDir(pad);
    for (const dir of ['up', 'down', 'left', 'right']) {
      if (pressed(pad, PAD[dir]) || stick === dir) down.add(dir);
    }
    for (const name of ['a', 'b', 'x', 'y', 'lb', 'rb', 'start', 'back']) {
      if (pressed(pad, PAD[name])) down.add(name);
    }
    if (down.size && !get(padUsed)) padUsed.set(true);
    for (const name of down) {
      if (!isOn()) continue;
      switch (name) {
        case 'up':
        case 'down':
        case 'left':
        case 'right':
          fire(name, now, true, () => regain() || moveFocus(name, doc));
          break;
        case 'a':
          fire(name, now, false, () => {
            if (regain()) return;
            const el = doc.activeElement;
            if (el && el !== doc.body) el.click();
          });
          break;
        case 'b':
          fire(name, now, false, () => {
            const target = doc.activeElement ?? doc.body;
            // Asked before Escape goes out: by the time it has, whatever was
            // open is closed, and B would close it and go back a page too.
            const open = !!doc.querySelector(LAYERS) || isTyping(target);
            const esc = new KeyboardEvent('keydown', { key: 'Escape', code: 'Escape', bubbles: true, cancelable: true });
            target.dispatchEvent(esc);
            // Nothing open to close: go back to where you came from.
            if (!open && !esc.defaultPrevented) actions.back?.();
          });
          break;
        case 'x':
          fire(name, now, false, () => actions.menu?.(doc.activeElement));
          break;
        case 'y':
        case 'start':
          fire(name, now, false, () => actions.palette?.());
          break;
        case 'lb':
          fire(name, now, false, () => actions.page?.(-1));
          break;
        case 'rb':
          fire(name, now, false, () => actions.page?.(1));
          break;
      }
    }
    for (const b of [...held.keys()]) if (!down.has(b)) held.delete(b);
  }
  const connect = () => {
    if (!frame) frame = win.requestAnimationFrame(poll);
  };
  win.addEventListener('gamepadconnected', connect);
  if ([...(win.navigator.getGamepads?.() ?? [])].some(Boolean)) connect();

  return () => {
    win.removeEventListener('keydown', onKey);
    doc.removeEventListener('focusin', onFocusIn);
    win.removeEventListener('gamepadconnected', connect);
    if (frame) win.cancelAnimationFrame(frame);
  };
}

/** The page after (or before) the one showing, round the sidebar. */
export function pageStep(current, delta) {
  const ids = PAGES.map((p) => p.id);
  const i = Math.max(0, ids.indexOf(current));
  return ids[(i + delta + ids.length) % ids.length];
}
