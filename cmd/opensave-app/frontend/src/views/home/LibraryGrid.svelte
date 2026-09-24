<script>
  // The tracked games, each saying where its save stands — as wide banners or
  // tall box art, as many to a row as chosen, filtered by name and by status.
  // "Select" turns the grid into a multi-select, to act on several games at
  // once (e.g. clear a batch of wrongly-tracked entries) without going in and
  // out of each one.
  import { navigate, toast, askConfirm } from '../../lib/stores.js';
  import { api } from '../../lib/api.js';
  import { SORTS } from '../../lib/gamestatus.js';
  import { libraryView, gridColumns, filterRows, offeredFilters } from '../../lib/libraryview.js';
  import { backdropClose } from '../../lib/backdrop.js';
  import FilterTabs from '../../components/ui/FilterTabs.svelte';
  import LibraryTile from './LibraryTile.svelte';
  import LibraryViewOptions from './LibraryViewOptions.svelte';

  /** [{game, status}] for every tracked game. */
  export let rows = [];

  let query = '';
  let status = 'all';

  $: filters = offeredFilters(rows);
  // A filter chosen while it made sense stays chosen only while it still does.
  $: if (status !== 'all' && !filters.some((f) => f.id === status)) status = 'all';
  $: shown = filterRows(rows, { query, status }).sort((a, b) => SORTS[$libraryView.sort].compare(a.game, b.game));
  $: filtering = query.trim() !== '' || status !== 'all';
  const clearFilters = () => {
    query = '';
    status = 'all';
  };

  let viewOpen = false;

  let selectMode = false;
  let libSelected = new Set();

  function toggleSelectMode() {
    selectMode = !selectMode;
    if (!selectMode) libSelected = new Set();
  }
  function toggleSelect(id) {
    if (libSelected.has(id)) libSelected.delete(id);
    else libSelected.add(id);
    libSelected = libSelected;
  }
  // "All" is what is on screen: selecting games a filter is hiding, to then
  // untrack them, would act on things nobody was looking at.
  $: allSelected = shown.length > 0 && shown.every((r) => libSelected.has(r.game.id));
  function toggleSelectAll() {
    libSelected = allSelected ? new Set() : new Set(shown.map((r) => r.game.id));
  }
  async function untrackSelected() {
    const n = libSelected.size;
    if (n === 0) return;
    const ok = await askConfirm(
      `Untrack ${n} selected game${n === 1 ? '' : 's'}? They'll be removed from your library. Your save snapshots on disk are kept — nothing is deleted.`,
      { title: 'Untrack selected?', confirmText: `Untrack ${n}`, danger: true }
    );
    if (!ok) return;
    try {
      const res = await api.post('/api/games/untrack-bulk', { ids: [...libSelected] });
      toast(`Untracked ${res.untracked} game${res.untracked === 1 ? '' : 's'} — snapshots kept`, 'success');
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      selectMode = false;
      libSelected = new Set();
    }
  }
</script>

<svelte:window on:keydown={(e) => e.key === 'Escape' && (viewOpen = false)} />

<div class="toolbar">
  <h3>Library</h3>
  <input class="search" type="search" placeholder="Find a game…" bind:value={query} aria-label="Find a game" />
  <div class="spacer"></div>
  {#if selectMode}
    <span class="select-count">{libSelected.size} selected</span>
    <button class="btn small" on:click={toggleSelectAll}>
      {allSelected ? 'Unselect all' : `Select all (${shown.length})`}
    </button>
    <button class="btn small danger" disabled={libSelected.size === 0} on:click={untrackSelected}>
      Untrack selected
    </button>
    <button class="btn small" on:click={toggleSelectMode}>Cancel</button>
  {:else}
    <label class="sort">
      <span>Sort</span>
      <select bind:value={$libraryView.sort}>
        {#each Object.entries(SORTS) as [id, s]}
          <option value={id}>{s.label}</option>
        {/each}
      </select>
    </label>
    <div class="view-anchor">
      <button class="btn small" class:active={viewOpen} aria-expanded={viewOpen} on:click={() => (viewOpen = !viewOpen)}>
        ▦ View
      </button>
      {#if viewOpen}
        <div class="view-backdrop" use:backdropClose={() => (viewOpen = false)} role="presentation"></div>
        <div class="view-panel card" role="dialog" aria-label="Library view">
          <LibraryViewOptions />
          <p class="view-hint">Also in Settings → General.</p>
        </div>
      {/if}
    </div>
    <button class="btn small" on:click={toggleSelectMode}>☑ Select</button>
  {/if}
</div>

{#if filters.length > 1}
  <div class="chips">
    <FilterTabs options={filters.map((f) => [f.id, f.label])} counts={Object.fromEntries(filters.map((f) => [f.id, f.count]))} bind:value={status} />
  </div>
{/if}

{#if shown.length === 0}
  <div class="none">
    <p>No games match{query.trim() ? ` "${query.trim()}"` : ''}.</p>
    <button class="btn small" on:click={clearFilters}>Clear filters</button>
  </div>
{:else}
  <div class="grid" style="grid-template-columns: {gridColumns($libraryView)}">
    {#each shown as { game, status: s } (game.id)}
      <LibraryTile
        {game}
        status={s}
        cover={$libraryView.cover}
        selecting={selectMode}
        selected={libSelected.has(game.id)}
        on:open={() => (selectMode ? toggleSelect(game.id) : navigate('game', { gameId: game.id }))}
      />
    {/each}
  </div>
  {#if filtering}
    <p class="showing">Showing {shown.length} of {rows.length}. <button class="linklike" on:click={clearFilters}>Show all</button></p>
  {/if}
{/if}

<style>
  .toolbar {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-bottom: 12px;
    flex-wrap: wrap;
  }
  .toolbar h3 {
    margin-right: 6px;
  }
  .search {
    width: 220px;
    padding: 6px 11px;
    background: var(--bg-raised);
    border: 1px solid var(--border-strong);
    border-radius: 8px;
    color: var(--text);
    font-size: 0.86rem;
    outline: none;
  }
  .search:focus {
    border-color: var(--accent);
  }
  .spacer {
    flex: 1;
  }
  .select-count {
    color: var(--text-dim);
    font-size: 0.85rem;
  }
  .sort {
    display: flex;
    align-items: center;
    gap: 7px;
    font-size: 0.82rem;
    color: var(--text-faint);
  }
  .sort select {
    padding: 5px 8px;
    background: var(--bg-raised);
    border: 1px solid var(--border-strong);
    border-radius: 8px;
    color: var(--text);
    font-size: 0.85rem;
  }
  .view-anchor {
    position: relative;
  }
  .btn.active {
    border-color: var(--accent);
  }
  .view-backdrop {
    position: fixed;
    inset: 0;
    z-index: 30;
  }
  .view-panel {
    position: absolute;
    right: 0;
    top: calc(100% + 8px);
    z-index: 31;
    width: 380px;
    max-width: calc(100vw - 32px);
    padding: 16px;
    box-shadow: 0 16px 40px rgba(0, 0, 0, 0.55);
  }
  .view-hint {
    margin-top: 12px;
    font-size: 0.75rem;
    color: var(--text-faint);
  }
  .chips {
    margin: -2px 0 14px;
  }
  .grid {
    display: grid;
    gap: 12px;
  }
  .none {
    text-align: center;
    color: var(--text-dim);
    padding: 40px 20px;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
  }
  .showing {
    margin-top: 12px;
    font-size: 0.8rem;
    color: var(--text-faint);
  }
  .linklike {
    background: none;
    border: none;
    padding: 0;
    font: inherit;
    color: var(--accent);
    cursor: pointer;
  }
</style>
