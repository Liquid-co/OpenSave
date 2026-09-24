<script>
  // The tracked games, each saying where its save stands — as wide banners or
  // tall box art, as many to a row as chosen, filtered by name and by status.
  // "Select" turns the grid into a multi-select, to act on several games at
  // once (e.g. clear a batch of wrongly-tracked entries) without going in and
  // out of each one.
  import { navigate } from '../../lib/stores.js';
  import { SORTS } from '../../lib/gamestatus.js';
  import { libraryView, gridColumns, filterRows, offeredFilters } from '../../lib/libraryview.js';
  import { backdropClose } from '../../lib/backdrop.js';
  import FilterTabs from '../../components/ui/FilterTabs.svelte';
  import LibraryTile from './LibraryTile.svelte';
  import LibraryViewOptions from './LibraryViewOptions.svelte';
  import { untrackGames } from '../../lib/gameactions.js';
  import { collections, collectionFilter, collectionChips, inCollection } from '../../lib/collections.js';
  import Star from 'lucide-svelte/icons/star';
  import LayoutGrid from 'lucide-svelte/icons/layout-grid';
  import SquareCheckBig from 'lucide-svelte/icons/square-check-big';

  /** [{game, status}] for every tracked game. */
  export let rows = [];

  let query = '';
  let status = 'all';

  $: filters = offeredFilters(rows);
  // A filter chosen while it made sense stays chosen only while it still does.
  $: if (status !== 'all' && !filters.some((f) => f.id === status)) status = 'all';
  // Collections narrow the list the same way the status chips do, and the
  // two combine: Favourites that need attention.
  $: chips = collectionChips($collections, rows);
  $: if ($collectionFilter && !$collections.some((c) => c.id === $collectionFilter)) collectionFilter.set('');
  $: shown = filterRows(inCollection(rows, $collections, $collectionFilter), { query, status }).sort((a, b) =>
    SORTS[$libraryView.sort].compare(a.game, b.game)
  );
  $: filtering = query.trim() !== '' || status !== 'all' || !!$collectionFilter;
  const clearFilters = () => {
    query = '';
    status = 'all';
    collectionFilter.set('');
  };
  const toggleCollection = (id) => collectionFilter.set($collectionFilter === id ? '' : id);

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
  function untrackSelected() {
    const picked = rows.filter((r) => libSelected.has(r.game.id)).map((r) => r.game);
    if (picked.length === 0) return;
    selectMode = false;
    libSelected = new Set();
    untrackGames(picked);
  }
</script>

<svelte:window on:keydown={(e) => e.key === 'Escape' && (viewOpen = false)} />

<div class="toolbar">
  <h3>Library</h3>
  <input class="search" type="search" data-find placeholder="Find a game…" bind:value={query} aria-label="Find a game" />
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
        <LayoutGrid size={14} />View
      </button>
      {#if viewOpen}
        <div class="view-backdrop" use:backdropClose={() => (viewOpen = false)} role="presentation"></div>
        <div class="view-panel card" role="dialog" aria-label="Library view">
          <LibraryViewOptions />
          <p class="view-hint">Also in Settings → General.</p>
        </div>
      {/if}
    </div>
    <button class="btn small" on:click={toggleSelectMode}><SquareCheckBig size={14} />Select</button>
  {/if}
</div>

{#if filters.length > 1 || chips.length}
  <div class="chips">
    {#if filters.length > 1}
      <FilterTabs options={filters.map((f) => [f.id, f.label])} counts={Object.fromEntries(filters.map((f) => [f.id, f.count]))} bind:value={status} />
    {/if}
    {#if chips.length}
      {#if filters.length > 1}<span class="chip-sep" aria-hidden="true"></span>{/if}
      <div class="collection-chips" role="group" aria-label="Collections">
        {#each chips as c}
          <button class="collection-chip" class:active={$collectionFilter === c.id} aria-pressed={$collectionFilter === c.id} on:click={() => toggleCollection(c.id)}>
            {#if c.builtin}<Star size={12} />{/if}{c.label}<span class="count">{c.count}</span>
          </button>
        {/each}
      </div>
    {/if}
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
    height: 28px;
    padding: 0 8px 0 10px;
    background-color: var(--bg-raised);
    border: 1px solid var(--border-strong);
    border-radius: 8px;
    color: var(--text);
    font-size: 0.85rem;
  }
  .view-anchor {
    position: relative;
  }
  .btn.active {
    background: var(--accent-soft);
    border-color: rgba(var(--accent-rgb), 0.45);
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
    box-shadow: var(--shadow);
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
  .chips {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 8px;
  }
  .chip-sep {
    width: 1px;
    height: 20px;
    background: var(--border-strong);
  }
  .collection-chips {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
  }
  .collection-chip {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 7px 13px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: transparent;
    color: var(--text-dim);
    font: inherit;
    font-size: 0.85rem;
    cursor: pointer;
  }
  .collection-chip:hover {
    background: var(--bg-hover);
  }
  .collection-chip.active {
    background: var(--accent-soft);
    border-color: rgba(var(--accent-rgb), 0.45);
    color: var(--text);
  }
  .collection-chip :global(svg) {
    color: var(--warn);
  }
  .collection-chip .count {
    font-size: 0.75rem;
    color: var(--text-faint);
  }
</style>
