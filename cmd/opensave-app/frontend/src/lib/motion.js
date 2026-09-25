// Whether things move: the Animations switch in Settings → General →
// Appearance, and never while the system asks apps to reduce motion.
//
// CSS reads the same answer from <html data-motion> (see applyAppearance and
// app.css). This is for Svelte's own transitions, which run from script and
// which CSS can only freeze, not skip: an outro stilled by CSS still keeps
// its element on screen for the length of the animation.
import { derived, get, readable } from 'svelte/store';
import { appearance, motionAllowed } from './appearance.js';

const query = globalThis.matchMedia?.('(prefers-reduced-motion: reduce)');

/** True while the system asks apps to reduce motion. */
export const systemReducesMotion = readable(!!query?.matches, (set) => {
  const onChange = () => set(!!query?.matches);
  query?.addEventListener?.('change', onChange);
  return () => query?.removeEventListener?.('change', onChange);
});

export const motionOn = derived([appearance, systemReducesMotion], ([a, reduce]) => motionAllowed(a, reduce));

/** A Svelte transition that happens only while things may move, and is
 *  instant otherwise. */
export const gated =
  (transition, isOn = () => get(motionOn)) =>
  (node, params) =>
    isOn() ? transition(node, params) : { duration: 0 };
