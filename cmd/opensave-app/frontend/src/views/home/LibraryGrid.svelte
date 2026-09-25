<script context="module">
  // The tiles come in one after another the first time the library is shown
  // in a session; after that the page's own arrival is enough, and coming
  // back to Home is not made to wait.
  let shownBefore = false;
</script>

<script>
  // The tracked games, each saying where its save stands — as wide banners or
  // tall box art, as many to a row as chosen, filtered by name and by status.
  // "Select" turns the grid into a multi-select, to act on several games at
  // once (e.g. clear a batch of wrongly-tracked entries) without going in and
  // out of each one.
  import { navigate, toast } from '../../lib/stores.js';
  import { api } from '../../lib/api.js';
  import Camera from 'lucide-svelte/icons/camera';
  import RefreshCw from 'lucide-svelte/icons/refresh-cw';
  import { SORTS, sortRows } from '../../lib/gamestatus.js';
  import ArrowDownWideNarrow from 'lucide-svelte/icons/arrow-down-wide-narrow';
  import ArrowUpNarrowWide from 'lucide-svelte/icons/arrow-up-narrow-wide';
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

  const firstShow = !shownBefore;
  shownBefore = true;

  let query = '';
  let status = 'all';

  $: filters = offeredFilters(rows);
  // A filter chosen while it made sense stays chosen only while it still does.
  $: if (status !== 'all' && !filters.some((f) => f.id === status)) status = 'all';
  // Collections narrow the list the same way the status chips do, and the
  // two combine: Favourites that need attention.
  $: chips = collectionChips($collections, rows);
  $: if ($collectionFilter && !$collections.some((c) => c.id === $collectionFilter)) collectionFilter.set('');
  $: shown = sortRows(filterRows(inCollection(rows, $collections, $collectionFilter), { query, status }), $libraryView.sort, $libraryView.reverse);
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

  // Selecting without the Select button: hold a tile down, or Ctrl-click it,
  // and the library is selecting with that game picked. While selecting, a
  // click toggles a game and Shift-click takes in everything from the last
  // one picked. Escape stops.
  let anchor = null;
  function startSelecting(id) {
    selectMode = true;
    libSelected = new Set([id]);
    anchor = id;
  }
  function tileLongPress(id) {
    if (!selectMode) startSelecting(id);
    else {
      toggleSelect(id);
      anchor = id;
    }
  }
  function tileOpen(id, mods = {}) {
    if (!selectMode) {
      if (mods.ctrlKey || mods.metaKey) startSelecting(id);
      else navigate('game', { gameId: id });
      return;
    }
    if (mods.shiftKey && anchor) {
      const order = shown.map((r) => r.game.id);
      const [from, to] = [order.indexOf(anchor), order.indexOf(id)].sort((a, b) => a - b);
      if (from >= 0) {
        for (const pick of order.slice(from, to + 1)) libSelected.add(pick);
        libSelected = libSelected;
        return;
      }
    }
    toggleSelect(id);
    anchor = id;
  }
  // The arrow keys move between games as they are laid out: left and right
  // along a row, up and down by a row's worth, Home and End to either end.
  // Enter opens, as it always did; while selecting it picks.
  function gridKeys(e) {
    const step = { ArrowRight: 1, ArrowLeft: -1, ArrowDown: 'down', ArrowUp: 'up', Home: 'first', End: 'last' }[e.key];
    if (step === undefined || e.altKey || e.ctrlKey || e.metaKey) return;
    const tiles = [...e.currentTarget.querySelectorAll('.tile')];
    const i = tiles.indexOf(document.activeElement);
    if (i < 0) return;
    const perRow = tiles.filter((t) => t.offsetTop === tiles[0].offsetTop).length || 1;
    const j =
      step === 'down' ? i + perRow : step === 'up' ? i - perRow : step === 'first' ? 0 : step === 'last' ? tiles.length - 1 : i + step;
    if (j < 0 || j >= tiles.length || j === i) return;
    e.preventDefault();
    tiles[j].focus();
    tiles[j].scrollIntoView({ block: 'nearest' });
  }

  function stopSelecting() {
    selectMode = false;
    libSelected = new Set();
    anchor = null;
  }

  // Things to do to all of them at once. The selection stays, so one can
  // follow another; untracking ends it.
  const picked = () => rows.filter((r) => libSelected.has(r.game.id)).map((r) => r.game);
  let bulkBusy = false;
  async function forEachPicked(verb, run) {
    const games = picked();
    if (games.length === 0 || bulkBusy) return;
    bulkBusy = true;
    const failed = [];
    for (const g of games) {
      try {
        await run(g);
      } catch (e) {
        failed.push(`${g.name}: ${e.message}`);
      }
    }
    bulkBusy = false;
    const done = games.length - failed.length;
    if (failed.length) toast(`${verb} ${done} of ${games.length} — ${failed.join('; ')}`, 'error');
    else toast(`${verb} ${done} ${done === 1 ? 'game' : 'games'}`, 'success');
  }
  const snapshotPicked = () => forEachPicked('Took a snapshot of', (g) => api.post(`/api/games/${g.id}/snapshot`, { comment: '' }));
  const syncPicked = () => forEachPicked('Started syncing', (g) => api.post(`/api/games/${g.id}/sync`, {}));
  const favouritePicked = () =>
    forEachPicked('Added to Favourites:', (g) => api.post('/api/collections/favourites/games', { gameId: g.id, in: true }));
  // "All" is what is on screen: selecting games a filter is hiding, to then
  // untrack them, would act on things nobody was looking at.
  $: allSelected = shown.length > 0 && shown.every((r) => libSelected.has(r.game.id));
  function toggleSelectAll() {
    libSelected = allSelected ? new Set() : new Set(shown.map((r) => r.game.id));
  }
  function untrackSelected() {
    const games = picked();
    if (games.length === 0) return;
    stopSelecting();
    untrackGames(games);
  }
</script>

<svelte:window
  on:keydown={(e) => {
    if (e.key !== 'Escape') return;
    if (viewOpen) viewOpen = false;
    else if (selectMode && !document.querySelector('.overlay, .backdrop[role="presentation"], .menu')) stopSelecting();
  }}
/>

<div class="toolbar">
  <h3>Library</h3>
  <input class="search" type="search" data-find placeholder="Find a game…" bind:value={query} aria-label="Find a game" />
  <div class="spacer"></div>
  {#if selectMode}
    <span class="select-count">{libSelected.size} selected</span>
    <button class="btn small" on:click={toggleSelectAll}>
      {allSelected ? 'Unselect all' : `Select all (${shown.length})`}
    </button>
    <button class="btn small" disabled={libSelected.size === 0 || bulkBusy} on:click={snapshotPicked} title="Take a snapshot of each">
      <Camera size={14} />Snapshot
    </button>
    <button class="btn small" disabled={libSelected.size === 0 || bulkBusy} on:click={syncPicked} title="Sync each with your devices">
      <RefreshCw size={14} />Sync
    </button>
    <button class="btn small" disabled={libSelected.size === 0 || bulkBusy} on:click={favouritePicked} title="Add each to Favourites">
      <Star size={14} />Favourite
    </button>
    <button class="btn small danger" disabled={libSelected.size === 0} on:click={untrackSelected}>
      Untrack selected
    </button>
    <button class="btn small" on:click={stopSelecting}>Cancel</button>
  {:else}
    <label class="sort">
      <span>Sort</span>
      <select bind:value={$libraryView.sort}>
        {#each Object.entries(SORTS) as [id, s]}
          <option value={id}>{s.label}</option>
        {/each}
      </select>
    </label>
    <button
      class="btn small icon order"
      on:click={() => libraryView.update((v) => ({ ...v, reverse: !v.reverse }))}
      title={$libraryView.reverse ? 'Reversed — put back in order' : 'Reverse the order'}
      aria-label="Reverse the order"
      aria-pressed={$libraryView.reverse}
    >
      <svelte:component this={$libraryView.reverse ? ArrowUpNarrowWide : ArrowDownWideNarrow} size={15} />
    </button>
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
  <div class="grid" style="grid-template-columns: {gridColumns($libraryView)}" on:keydown={gridKeys} role="presentation">
    {#each shown as { game, status: s }, i (game.id)}
      <LibraryTile
        {game}
        status={s}
        enter={firstShow ? i : -1}
        cover={$libraryView.cover}
        selecting={selectMode}
        selected={libSelected.has(game.id)}
        on:open={(e) => tileOpen(game.id, e.detail)}
        on:longpress={() => tileLongPress(game.id)}
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
