<script>
  import { t as translateUI } from '../../lib/i18n.js';
  // The Steam App ID box, which says what Steam calls the number as it is
  // typed — so "did I get the number right?" is answered before Save rather
  // than by whether a cover eventually appears. A cover that does not appear
  // says nothing about why: a typo, a game Steam has no art for, and a
  // network that blocks Steam all look identical from the outside, which is
  // how "I added the app ID but it doesn't do anything" gets asked.
  import { onDestroy } from 'svelte';
  import { api } from '../../lib/api.js';
  import MoreInfo from '../../components/ui/MoreInfo.svelte';
  import CircleCheck from 'lucide-svelte/icons/circle-check';

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
  <label for="c-appid">{$translateUI('appIdField.steamAppID')}</label>
  <input id="c-appid" placeholder="e.g. 1091500" bind:value />
  <span class="hint">
    {$translateUI('appIdField.theNumberInTheGameSSteamStore')}
    <MoreInfo>
      {$translateUI('appIdField.withMatchGamesBySteamAppID')} <code>AppData</code> {$translateUI('appIdField.oftenHasNothingToWorkItOut')}
    </MoreInfo>
  </span>
  {#if error}<span class="hint hint-error">{error}</span>{/if}
  {#if check?.state === 'checking'}
    <span class="hint appid-check">{$translateUI('appIdField.checkingWithSteam')}</span>
  {:else if check?.state === 'found'}
    <span class="hint appid-check ok"><CircleCheck size={13} class="inline-icon" />{$translateUI('appIdField.steam')} <strong>{check.name}</strong></span>
  {:else if check?.state === 'unknown'}
    <span class="hint appid-check warn">
      {$translateUI('appIdField.noSteamGameHasThisAppID')}
      <code>store.steampowered.com/app/<strong>1091500</strong>/…</code> {$translateUI('appIdField.notTheOneFromAnotherStore')}
    </span>
  {:else if check?.state === 'unreachable'}
    <span class="hint appid-check warn">
      {$translateUI('appIdField.couldnTReachSteamToCheckThisNumber')}
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
    color: var(--success);
  }
  .appid-check.warn {
    color: var(--warn);
  }
  .hint-error {
    color: var(--danger);
  }
</style>
