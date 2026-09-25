import './app.css';
import App from './App.svelte';
import { appearance, applyAppearance } from './lib/appearance.js';
import { native } from './lib/api.js';
// Registers where collection lists from the daemon go; must be in place
// before the first state arrives (see onCollections in lib/stores.js).
import './lib/collections.js';

// Before the first paint, so a light or rescaled window never starts out
// dark and normal-sized; then again on every change, from Settings or from
// the system when following it.
const dark = globalThis.matchMedia?.('(prefers-color-scheme: dark)');
const reduce = globalThis.matchMedia?.('(prefers-reduced-motion: reduce)');
let current;
const apply = () =>
  applyAppearance(current, {
    prefersDark: dark ? dark.matches : true,
    reduceMotion: !!reduce?.matches,
    setWindowBackground: native.setWindowBackground
  });
appearance.subscribe((v) => {
  current = v;
  apply();
});
dark?.addEventListener?.('change', apply);
reduce?.addEventListener?.('change', apply);

const app = new App({ target: document.getElementById('app') });

export default app;
