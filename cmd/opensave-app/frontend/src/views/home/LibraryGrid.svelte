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
  import { t } from '../../lib/i18n.js';
  import { api } from '../../lib/api.js';
  import Camera from 'lucide-svelte/icons/camera';
  import RefreshCw from 'lucide-svelte/icons/refresh-cw';
  import { SORTS, sortRows } from '../../lib/gamestatus.js';
  import ArrowDownWideNarrow from 'lucide-svelte/icons/arrow-down-wide-narrow';
  import ArrowUpNarrowWide from 'lucide-svelte/icons/arrow-up-narrow-wide';
  import { libraryView, gridColumns, filterRows, offeredFilters } from '../../lib/libraryview.js';
  import Dropdown from '../../components/ui/Dropdown.svelte';
  import { gatedFlip, pop, slidingIndicator, unfold } from '../../lib/motion.js';
  import { fade } from 'svelte/transition';
  import LibraryTile from './LibraryTile.svelte';
  import LibraryViewOptions from './LibraryViewOptions.svelte';
  import { untrackGames } from '../../lib/gameactions.js';
  import { collections, collectionFilter, collectionChips, inCollection } from '../../lib/collections.js';
  import Star from 'lucide-svelte/icons/star';
  import LayoutGrid from 'lucide-svelte/icons/layout-grid';
  import SquareCheckBig from 'lucide-svelte/icons/square-check-big';
  import Search from 'lucide-svelte/icons/search';
  import ChevronDown from 'lucide-svelte/icons/chevron-down';
  import X from 'lucide-svelte/icons/x';

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

  // The order, as a menu rather than a list box: each way to sort, the one in
  // use ticked, and reversing it at the foot.
  let sortOpen = false;
  let sortAnchor;
  let sortWidth = 0;
  $: sortItems = [
    ...Object.keys(SORTS).map((id) => ({
      label: $t('game.sort.' + id),
      checked: $libraryView.sort === id,
      run: () => libraryView.update((v) => ({ ...v, sort: id }))
    })),
    null,
    {
      label: $t('home.library.reverseTitle'),
      checked: $libraryView.reverse,
      run: () => libraryView.update((v) => ({ ...v, reverse: !v.reverse }))
    }
  ];
  let viewAnchor;
  // A press outside either menu closes it; one inside, or on its own button,
  // is left to them.
  const sortOutside = (e) => {
    if (sortOpen && !sortAnchor?.contains(e.target)) sortOpen = false;
    if (viewOpen && !viewAnchor?.contains(e.target)) viewOpen = false;
  };

  // Moving tiles to their new places is measured per tile; past this many it
  // is skipped and the grid simply redraws.
  const FLIP_LIMIT = 150;

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

  // From the Library's Sync button: "Choose games to sync" starts selecting
  // with nothing picked yet, and brings the grid's toolbar into view.
  let toolbar;
  export function chooseToSync() {
    selectMode = true;
    libSelected = new Set();
    anchor = null;
    toolbar?.scrollIntoView({ block: 'start', behavior: 'smooth' });
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
    if (failed.length) toast($t('home.library.bulkPartial', { verb, done, total: games.length, failed: failed.join('; ') }), 'error');
    else toast($t('home.library.bulkDone', { verb, n: done }), 'success');
  }
  const snapshotPicked = () =>
    forEachPicked($t('home.library.verbSnapshot'), (g) => api.post(`/api/games/${g.id}/snapshot`, { comment: '' }));
  const syncPicked = () => forEachPicked($t('home.library.verbSync'), (g) => api.post(`/api/games/${g.id}/sync`, {}));
  const favouritePicked = () =>
    forEachPicked($t('home.library.verbFavourite'), (g) => api.post('/api/collections/favourites/games', { gameId: g.id, in: true }));
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
  on:mousedown={sortOutside}
  on:keydown={(e) => {
    if (e.key !== 'Escape') return;
    if (viewOpen) viewOpen = false;
    else if (selectMode && !document.querySelector('.overlay, .backdrop[role="presentation"], .menu')) stopSelecting();
  }}
/>

<div class="toolbar" bind:this={toolbar}>
  <h2 class="section-title">{$t('home.library.allGames')}</h2>

  <!-- Which games: by where their save stands, then by collection. One row
       of pills, the collections after a rule. -->
  <div class="tabs" role="group" aria-label={$t('home.library.filterLabel')}>
    <div class="pills" use:slidingIndicator={{ mode: 'fill', className: 'pill-mark' }}>
      {#each filters as f (f.id)}
        <button class="tab" class:on={status === f.id} aria-pressed={status === f.id} on:click={() => (status = f.id)}>
          {$t('game.statusFilter.' + f.id)}<span class="n">{f.count}</span>
        </button>
      {/each}
    </div>
    {#if chips.length}
      <span class="rule" aria-hidden="true"></span>
      <div class="pills" use:slidingIndicator={{ mode: 'fill', className: 'pill-mark' }}>
        {#each chips as c (c.id)}
          <button class="tab" class:on={$collectionFilter === c.id} aria-pressed={$collectionFilter === c.id} on:click={() => toggleCollection(c.id)}>
            {#if c.builtin}<Star size={12} class="star" />{/if}{c.label}<span class="n">{c.count}</span>
          </button>
        {/each}
      </div>
    {/if}
  </div>

  <span class="spacer"></span>

  {#if selectMode}
    <div class="selection" role="group" aria-label={$t('home.library.selectedCount', { n: libSelected.size })} in:pop={{ start: 0.97 }}>
      <button class="tool" on:click={stopSelecting} title={$t('common.cancel')} aria-label={$t('common.cancel')}><X size={15} /></button>
      <span class="count-sel">{#key libSelected.size}<span class="num" in:pop={{ start: 0.8, duration: 160 }}>{$t('home.library.selectedCount', { n: libSelected.size })}</span>{/key}</span>
      <button class="tool" on:click={toggleSelectAll}>
        {allSelected ? $t('home.library.unselectAll') : $t('home.library.selectAll', { count: shown.length })}
      </button>
      <span class="rule" aria-hidden="true"></span>
      <button class="tool" disabled={libSelected.size === 0 || bulkBusy} on:click={syncPicked} title={$t('home.library.syncTitle')}>
        <RefreshCw size={15} />{$t('home.library.sync')}
      </button>
      <button class="tool" disabled={libSelected.size === 0 || bulkBusy} on:click={snapshotPicked} title={$t('home.library.snapshotTitle')}>
        <Camera size={15} />{$t('home.library.snapshot')}
      </button>
      <button class="tool" disabled={libSelected.size === 0 || bulkBusy} on:click={favouritePicked} title={$t('home.library.favouriteTitle')}>
        <Star size={15} />{$t('home.library.favourite')}
      </button>
      <button class="tool danger" disabled={libSelected.size === 0} on:click={untrackSelected}>
        {$t('home.library.untrackSelected')}
      </button>
    </div>
  {:else}
    <div class="tools" in:fade={{ duration: 140 }}>
    <label class="find">
      <Search size={14} />
      <input type="search" data-find placeholder={$t('home.library.searchPlaceholder')} bind:value={query} aria-label={$t('home.library.searchLabel')} />
    </label>
    <div class="anchor" bind:this={sortAnchor} bind:clientWidth={sortWidth}>
      <button
        class="tool"
        class:on={sortOpen}
        on:click={() => (sortOpen = !sortOpen)}
        title="{$t('home.library.sort')}: {$t('game.sort.' + $libraryView.sort)}"
        aria-haspopup="menu"
        aria-expanded={sortOpen}
      >
        <svelte:component this={$libraryView.reverse ? ArrowUpNarrowWide : ArrowDownWideNarrow} size={15} />
        {$t('game.sort.' + $libraryView.sort)}
        <span class="chev"><ChevronDown size={13} /></span>
      </button>
      <Dropdown items={sortItems} bind:open={sortOpen} minWidth={sortWidth} label={$t('home.library.sort')} />
    </div>
    <div class="anchor" bind:this={viewAnchor}>
      <button class="tool" class:on={viewOpen} aria-expanded={viewOpen} aria-haspopup="dialog" on:click={() => (viewOpen = !viewOpen)}>
        <LayoutGrid size={15} />{$t('home.library.view')}
        <span class="chev"><ChevronDown size={13} /></span>
      </button>
      {#if viewOpen}
        <div class="view-panel" role="dialog" aria-label={$t('home.library.viewLabel')} transition:unfold={{ origin: 'top right' }}>
          <div class="view-head">
            <span class="view-title">{$t('home.library.viewLabel')}</span>
            <span class="view-hint">{$t('home.library.viewHint')}</span>
          </div>
          <LibraryViewOptions />
        </div>
      {/if}
    </div>
    <button class="tool" on:click={toggleSelectMode}><SquareCheckBig size={15} />{$t('home.library.select')}</button>
    </div>
  {/if}
</div>

{#if shown.length === 0}
  <div class="none">
    {#if query.trim()}
      <p>{$t('home.library.emptyFor', { query: query.trim() })}</p>
    {:else}
      <p>{$t('home.library.empty')}</p>
    {/if}
    <button class="btn small" on:click={clearFilters}>{$t('home.library.clearFilters')}</button>
  </div>
{:else}
  <div class="grid" style="grid-template-columns: {gridColumns($libraryView)}" on:keydown={gridKeys} role="presentation">
    {#each shown as { game, status: s }, i (game.id)}
      <div class="cell" animate:gatedFlip={{ count: shown.length, limit: FLIP_LIMIT }} in:pop={{ delay: 40 }} out:pop={{ duration: 120 }}>
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
      </div>
    {/each}
  </div>
  {#if filtering}
    <p class="showing">
      {$t('home.library.showing', { shown: shown.length, total: rows.length })}
      <button class="linklike" on:click={clearFilters}>{$t('home.library.showAll')}</button>
    </p>
  {/if}
{/if}

<style>
  .toolbar {
    display: flex;
    align-items: center;
    gap: 8px 10px;
    margin-bottom: 16px;
    flex-wrap: wrap;
    scroll-margin-top: 12px;
  }
  .section-title {
    font-size: var(--text-lg);
    font-weight: 600;
    margin-right: 4px;
  }
  .spacer {
    flex: 1;
  }

  /* The filter pills, the same look as FilterTabs. */
  .tabs {
    display: flex;
    align-items: center;
    gap: 4px;
    flex-wrap: wrap;
  }
  .tab {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 30px;
    padding: 0 12px;
    border: 1px solid transparent;
    border-radius: 999px;
    background: transparent;
    color: var(--text-dim);
    font: inherit;
    font-size: var(--text-md);
    white-space: nowrap;
    cursor: pointer;
  }
  .tab:hover {
    background: var(--bg-hover);
    color: var(--text);
  }
  .tab.on {
    background: var(--bg-active);
    border-color: var(--border-strong);
    color: var(--text);
    font-weight: 600;
  }
  /* With the marker sliding (slidingIndicator), it draws the chosen pill
     and moves between them; the pills put their own fill away. */
  .pills {
    display: flex;
    align-items: center;
    gap: 4px;
  }
  .pills:global(.slides) .tab {
    position: relative;
    z-index: 1;
  }
  .pills:global(.slides) .tab.on {
    background: transparent;
    border-color: transparent;
  }
  .pills :global(.pill-mark) {
    background: var(--bg-active);
    border: 1px solid var(--border-strong);
    border-radius: 999px;
  }
  .tab {
    transition:
      background 0.12s,
      color 0.12s;
  }
  .n {
    font-size: var(--text-sm);
    font-weight: 400;
    color: var(--text-faint);
  }
  .tab :global(.star) {
    color: var(--warn);
  }
  .rule {
    width: 1px;
    height: 18px;
    margin: 0 6px;
    background: var(--border-strong);
  }

  /* The tools on the right: quiet until used. */
  .find {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 190px;
    height: 30px;
    padding: 0 10px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--bg-raised);
    color: var(--text-faint);
    cursor: text;
    transition: width 0.15s ease;
  }
  .find:focus-within {
    width: 240px;
    border-color: var(--border-strong);
    color: var(--text-dim);
  }
  .find input {
    flex: 1;
    min-width: 0;
    border: none;
    outline: none;
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: var(--text-md);
  }
  .tool {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    height: 30px;
    padding: 0 10px;
    border: 1px solid transparent;
    border-radius: 8px;
    background: transparent;
    color: var(--text-dim);
    font: inherit;
    font-size: var(--text-md);
    font-weight: 500;
    white-space: nowrap;
    cursor: pointer;
  }
  .tool:hover:not(:disabled),
  .tool.on,
  .tool:global([data-menu-open]) {
    background: var(--bg-hover);
    color: var(--text);
  }
  .tool:disabled {
    opacity: 0.45;
    cursor: default;
  }
  .tools {
    display: flex;
    align-items: center;
    gap: 8px 10px;
    flex-wrap: wrap;
  }
  .anchor {
    position: relative;
  }
  .chev {
    display: inline-flex;
    color: var(--text-faint);
    margin-left: -2px;
    transition: transform 0.18s cubic-bezier(0.2, 0.7, 0.2, 1);
  }
  .tool.on .chev {
    transform: rotate(180deg);
  }
  .tool {
    transition:
      background 0.12s,
      color 0.12s;
  }
  .count-sel .num {
    display: inline-block;
  }
  .tool.danger {
    color: var(--danger-text);
  }
  .tool.danger:hover:not(:disabled) {
    background: rgba(var(--danger-rgb), 0.14);
  }

  /* Selecting: the tools give way to what can be done to the games picked,
     on a tinted bar, so it is plain the grid is in another mode. */
  .selection {
    display: flex;
    align-items: center;
    gap: 2px;
    padding: 3px 6px 3px 3px;
    border-radius: 10px;
    background: var(--accent-soft);
    border: 1px solid rgba(var(--accent-rgb), 0.35);
    flex-wrap: wrap;
  }
  .selection .tool {
    color: var(--text);
  }
  .selection .tool:hover:not(:disabled) {
    background: rgba(var(--accent-rgb), 0.18);
  }
  .count-sel {
    padding: 0 8px 0 4px;
    font-weight: 600;
    font-size: var(--text-md);
  }
  .selection .rule {
    background: rgba(var(--accent-rgb), 0.35);
    margin: 0 4px;
  }

  .view-panel {
    position: absolute;
    right: 0;
    top: calc(100% + 6px);
    z-index: 70;
    width: 380px;
    max-width: calc(100vw - 32px);
    padding: 16px 16px 14px;
    background: var(--bg-raised);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow);
  }
  .view-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 12px;
    margin-bottom: 14px;
  }
  .view-title {
    font-size: var(--text-lg);
    font-weight: 600;
  }
  .view-hint {
    font-size: var(--text-sm);
    color: var(--text-faint);
  }
  .grid {
    display: grid;
    gap: 12px;
  }
  /* Each tile's own cell, so it can be moved and faded as one. */
  .cell {
    display: grid;
    min-width: 0;
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
    color: var(--accent-text);
    cursor: pointer;
  }
</style>
