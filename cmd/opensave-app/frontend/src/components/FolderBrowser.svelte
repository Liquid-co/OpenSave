<script>
  // A folder navigator, for where there is no system folder picker: the
  // browser build. The desktop app opens the system's own picker instead.
  // Folders come from the daemon (GET /api/folders), folders only.
  //
  // Opens at `start` when that is a folder, otherwise at the places saves are
  // usually under. Fires `pick` with the folder chosen.
  import { createEventDispatcher, onMount } from 'svelte';
  import Folder from 'lucide-svelte/icons/folder';
  import ArrowUp from 'lucide-svelte/icons/arrow-up';
  import HardDrive from 'lucide-svelte/icons/hard-drive';
  import Home from 'lucide-svelte/icons/house';
  import Check from 'lucide-svelte/icons/check';
  import { api } from '../lib/api.js';
  import { t } from '../lib/i18n.js';
  import { rise } from '../lib/motion.js';

  export let start = '';
  const dispatch = createEventDispatcher();

  let places = [];
  let drives = [];
  let listing = null; // { path, parent, folders, more }
  let loading = false;
  let error = '';
  let list;

  async function open(path) {
    loading = true;
    error = '';
    try {
      listing = await api.get(`/api/folders?path=${encodeURIComponent(path)}`);
      list?.scrollTo?.(0, 0);
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  onMount(async () => {
    try {
      ({ places = [], drives = [] } = await api.get('/api/folders'));
    } catch (e) {
      error = e.message;
    }
    const from = String(start ?? '').trim();
    if (from) await open(from);
    if (!listing && places[0]) {
      error = '';
      await open(places[0].path);
    }
  });

  function key(e) {
    if (e.key === 'Backspace' && listing?.parent && e.target?.tagName !== 'INPUT') {
      e.preventDefault();
      open(listing.parent);
    }
  }
</script>

<!-- svelte-ignore a11y-no-noninteractive-element-interactions -->
<div class="browser" role="group" aria-label={$t('folders.title')} on:keydown={key} in:rise={{ y: 4 }}>
  <div class="places" aria-label={$t('folders.places')}>
    {#each places as p}
      <button class="place" class:on={listing?.path === p.path} on:click={() => open(p.path)}>
        {#if p.name === 'home'}<Home size={13} />{/if}{$t('folders.place.' + p.name)}
      </button>
    {/each}
    {#each drives as d}
      <button class="place" class:on={listing?.path === d.path} on:click={() => open(d.path)}><HardDrive size={13} />{d.name}</button>
    {/each}
  </div>

  <div class="here">
    <button class="up" disabled={!listing?.parent || loading} title={$t('folders.up')} aria-label={$t('folders.up')} on:click={() => open(listing.parent)}><ArrowUp size={15} /></button>
    <span class="path" title={listing?.path}><bdi>{listing?.path ?? ''}</bdi></span>
    <button class="btn small primary" disabled={!listing || loading} on:click={() => dispatch('pick', listing.path)}><Check size={14} />{$t('folders.useThis')}</button>
  </div>

  {#if error}<p class="error" role="alert">{error}</p>{/if}

  <div class="list" class:dim={loading} bind:this={list}>
    {#if listing}
      {#each listing.folders as f (f.path)}
        <button class="folder" on:click={() => open(f.path)}><Folder size={15} /><span>{f.name}</span></button>
      {:else}
        <p class="empty">{$t('folders.empty')}</p>
      {/each}
      {#if listing.more}<p class="empty">{$t('folders.more', { n: listing.folders.length })}</p>{/if}
    {:else if loading}
      <p class="empty">{$t('folders.loading')}</p>
    {/if}
  </div>
</div>

<style>
  .browser {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 10px;
    border-radius: 12px;
    border: 1px solid var(--border-strong);
    background: var(--bg);
  }
  .places {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
  .place {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    height: 26px;
    padding: 0 10px;
    border-radius: 999px;
    border: 1px solid var(--border);
    background: transparent;
    color: var(--text-dim);
    font: inherit;
    font-size: var(--text-sm);
    cursor: pointer;
  }
  .place:hover {
    border-color: var(--border-strong);
    color: var(--text);
  }
  .place.on {
    background: var(--bg-active);
    border-color: var(--border-strong);
    color: var(--text);
    font-weight: 600;
  }
  .here {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }
  .up {
    width: 30px;
    height: 30px;
    flex-shrink: 0;
    display: grid;
    place-items: center;
    border-radius: 8px;
    border: 1px solid var(--border);
    background: var(--bg-raised);
    color: var(--text-dim);
    cursor: pointer;
  }
  .up:hover:not(:disabled) {
    color: var(--text);
    border-color: var(--border-strong);
  }
  .up:disabled {
    opacity: 0.4;
    cursor: default;
  }
  /* The end of a path names the folder, so a long one keeps its end. */
  .path {
    flex: 1;
    min-width: 0;
    font-family: var(--font-mono, ui-monospace, monospace);
    font-size: var(--text-sm);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    direction: rtl;
    text-align: left;
  }
  .list {
    display: flex;
    flex-direction: column;
    height: 216px;
    overflow-y: auto;
    border-top: 1px solid var(--border);
    padding-top: 6px;
    transition: opacity 0.12s;
  }
  .list.dim {
    opacity: 0.55;
  }
  .folder {
    display: flex;
    align-items: center;
    gap: 9px;
    min-height: 32px;
    padding: 0 8px;
    border: none;
    border-radius: 8px;
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: var(--text-md);
    text-align: left;
    cursor: pointer;
  }
  .folder :global(svg) {
    flex-shrink: 0;
    color: var(--accent-text);
  }
  .folder span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .folder:hover,
  .folder:focus-visible {
    background: var(--bg-hover);
  }
  .empty {
    padding: 14px 8px;
    color: var(--text-faint);
    font-size: var(--text-sm);
  }
  .error {
    color: var(--danger, #e5484d);
    font-size: var(--text-sm);
  }
</style>
