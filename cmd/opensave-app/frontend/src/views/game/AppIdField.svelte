<script>
  // The Steam App ID box, which says what Steam calls the number as it is
  // typed — so "did I get the number right?" is answered before Save rather
  // than by whether a cover eventually appears. A cover that does not appear
  // says nothing about why: a typo, a game Steam has no art for, and a
  // network that blocks Steam all look identical from the outside, which is
  // how "I added the app ID but it doesn't do anything" gets asked.
  import { onDestroy } from 'svelte';
  import { api } from '../../lib/api.js';

  export let value = '';
  /** Set by the form when Save refuses the value. */
  export let error = '';

  // Three states, kept apart because each wants different advice.
  let check = null; // null | { state: 'checking'|'found'|'unknown'|'unreachable', name? }
  let timer = null;
  let checkedFor = '';
  onDestroy(() => clearTimeout(timer));

  $: schedule(value ?? '');

  function schedule(raw) {
    const id = String(raw).trim();
    clearTimeout(timer);
    if (id === '' || !/^\d+$/.test(id)) {
      check = null;
      checkedFor = '';
      return;
    }
    if (id === checkedFor) return;
    // Debounced: a lookup per keystroke would hit Steam for every partial
    // number on the way to the real one.
    timer = setTimeout(() => lookup(id), 500);
  }

  async function lookup(id) {
    check = { state: 'checking' };
    try {
      const res = await api.get(`/api/steam/app?appId=${encodeURIComponent(id)}`);
      // The field may have moved on while Steam was answering.
      if (String(value ?? '').trim() !== id) return;
      checkedFor = id;
      check = res?.found
        ? { state: 'found', name: res.name }
        : { state: res?.reason === 'unreachable' ? 'unreachable' : 'unknown' };
    } catch {
      if (String(value ?? '').trim() !== id) return;
      checkedFor = id;
      check = { state: 'unreachable' };
    }
  }
</script>

<div class="field">
  <label for="c-appid">Steam App ID</label>
  <input id="c-appid" placeholder="e.g. 1091500" bind:value />
  <span class="hint">
    Launches via Steam, fetches cover art, and — with “Match games by Steam App ID”
    turned on in Settings — lets this game sync with a copy tracked under a different
    name on another device. Filled in automatically when it can be worked out; a save
    under <code>AppData</code> often has nothing to work it out from, so you can set it
    here. It's the number in the game's Steam store URL.
  </span>
  {#if error}<span class="hint hint-error">{error}</span>{/if}
  {#if check?.state === 'checking'}
    <span class="hint appid-check">Checking with Steam…</span>
  {:else if check?.state === 'found'}
    <span class="hint appid-check ok">✓ Steam: <strong>{check.name}</strong></span>
  {:else if check?.state === 'unknown'}
    <span class="hint appid-check warn">
      No Steam game has this App ID. It's the number in the store page address —
      <code>store.steampowered.com/app/<strong>1091500</strong>/…</code> — not the one
      from another store.
    </span>
  {:else if check?.state === 'unreachable'}
    <span class="hint appid-check warn">
      Couldn't reach Steam to check this number. If Steam's store or CDN is blocked on
      this network, cover art won't load here either — the ID itself may well be right.
    </span>
  {/if}
</div>

<style>
  .field {
    margin-bottom: 14px;
  }
  .appid-check {
    display: block;
    margin-top: 4px;
  }
  .appid-check.ok {
    color: var(--ok-fg, #1a7f4b);
  }
  .appid-check.warn {
    color: var(--warn-fg, #8a6100);
  }
  .hint-error {
    color: var(--danger);
  }
</style>
