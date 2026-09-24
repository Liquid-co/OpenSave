<script>
  // The tracked games, as cover cards, each saying where its save stands.
  // "Select" turns the grid into a multi-select, to act on several games at
  // once (e.g. clear a batch of wrongly-tracked entries) without going in and
  // out of each one.
  import { navigate, toast, askConfirm } from '../../lib/stores.js';
  import { api, gameCover, isDaemonURL } from '../../lib/api.js';
  import { SORTS, latestSnapshotAt } from '../../lib/gamestatus.js';
  import CoverImage from '../../components/CoverImage.svelte';

  /** [{game, status}] for every tracked game. */
  export let rows = [];

  // The chosen order is remembered on this device only; nothing depends on it.
  const SORT_KEY = 'opensave.librarySort';
  let sort = 'name';
  try {
    const saved = localStorage.getItem(SORT_KEY);
    if (saved && SORTS[saved]) sort = saved;
  } catch {}
  $: try {
    localStorage.setItem(SORT_KEY, sort);
  } catch {}

  $: sorted = [...rows].sort((a, b) => SORTS[sort].compare(a.game, b.game));

  // Explicit covers are blurred until the pointer is on their card.
  let revealed = null;

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
  $: allSelected = rows.length > 0 && libSelected.size === rows.length;
  function toggleSelectAll() {
    libSelected = allSelected ? new Set() : new Set(rows.map((r) => r.game.id));
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

  const snapshotCount = (game) =>
    Object.values(game.branches ?? {}).reduce((n, b) => n + (b.snapshots?.length ?? 0), 0);
</script>

<div class="section-row">
  <h3>Library</h3>
  {#if selectMode}
    <div class="bar">
      <span class="select-count">{libSelected.size} selected</span>
      <button class="btn small" on:click={toggleSelectAll}>
        {allSelected ? 'Unselect all' : `Select all (${rows.length})`}
      </button>
      <button class="btn small danger" disabled={libSelected.size === 0} on:click={untrackSelected}>
        Untrack selected
      </button>
      <button class="btn small" on:click={toggleSelectMode}>Cancel</button>
    </div>
  {:else}
    <div class="bar">
      <label class="sort">
        <span>Sort</span>
        <select bind:value={sort}>
          {#each Object.entries(SORTS) as [id, s]}
            <option value={id}>{s.label}</option>
          {/each}
        </select>
      </label>
      <button class="btn small" on:click={toggleSelectMode}>☑ Select</button>
    </div>
  {/if}
</div>
<div class="grid">
  {#each sorted as { game, status } (game.id)}
    {@const cover = gameCover(game)}
    <button
      class="card game-card"
      class:selected={selectMode && libSelected.has(game.id)}
      title={game.savePath}
      on:click={() => (selectMode ? toggleSelect(game.id) : navigate('game', { gameId: game.id }))}
      on:mouseenter={() => (revealed = game.id)}
      on:mouseleave={() => (revealed = null)}
    >
      {#if selectMode}
        <div class="tick" class:on={libSelected.has(game.id)}>{libSelected.has(game.id) ? '✓' : ''}</div>
      {/if}
      <div class="cover">
        {#if cover && isDaemonURL(cover)}
          <CoverImage src={cover} revealed={revealed === game.id} />
        {:else if cover}
          <img src={cover} alt="" loading="lazy" on:error={(e) => (e.currentTarget.style.display = 'none')} />
        {/if}
        <div class="cover-fallback"><span>{game.name}</span></div>
      </div>
      <div class="body">
        <div class="name">{game.name}</div>
        <div class="status tone-{status.tone}">
          <span class="dot"></span>{status.label}
        </div>
        <div class="meta">
          {snapshotCount(game)} {snapshotCount(game) === 1 ? 'snapshot' : 'snapshots'}
          {#if game.activeBranch && game.activeBranch !== 'main'}
            · on <strong>{game.activeBranch}</strong>
          {/if}
        </div>
      </div>
    </button>
  {/each}
</div>

<style>
  .section-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    margin-bottom: 12px;
    flex-wrap: wrap;
  }
  .bar {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
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
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
    gap: 12px;
  }
  .game-card {
    position: relative;
    text-align: left;
    cursor: pointer;
    color: var(--text);
    transition: border-color 0.12s, transform 0.12s;
    padding: 0;
    overflow: hidden;
  }
  .game-card:hover {
    border-color: var(--border-strong);
    transform: translateY(-1px);
  }
  .game-card.selected {
    border-color: var(--accent);
    box-shadow: 0 0 0 1px var(--accent);
  }
  .tick {
    position: absolute;
    top: 8px;
    left: 8px;
    z-index: 3;
    width: 24px;
    height: 24px;
    border-radius: 50%;
    background: rgba(0, 0, 0, 0.55);
    border: 2px solid #fff;
    color: #fff;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 0.8rem;
    font-weight: 700;
    box-shadow: 0 2px 6px rgba(0, 0, 0, 0.4);
  }
  .tick.on {
    background: var(--accent);
    border-color: var(--accent);
  }
  .cover {
    position: relative;
    aspect-ratio: 460 / 175;
    background: var(--bg);
    border-bottom: 1px solid var(--border);
    overflow: hidden;
  }
  .cover :global(img) {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
    z-index: 1;
  }
  .cover-fallback {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 12px;
    background: linear-gradient(135deg, rgba(138, 99, 244, 0.16), rgba(138, 99, 244, 0.04));
  }
  .cover-fallback span {
    font-weight: 700;
    font-size: 1.05rem;
    color: var(--text-dim);
    text-align: center;
    overflow: hidden;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
  }
  .body {
    padding: 12px 16px 14px;
  }
  .name {
    font-weight: 600;
    margin-bottom: 5px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  /* Where the save stands, in the colour that says whether to look. */
  .status {
    --tone: var(--success);
    display: flex;
    align-items: center;
    gap: 7px;
    font-size: 0.8rem;
    color: var(--text);
    margin-bottom: 4px;
  }
  .status.tone-warn {
    --tone: var(--warn);
    color: var(--warn);
  }
  .status.tone-busy {
    --tone: var(--accent);
    color: var(--accent);
  }
  .status.tone-muted {
    --tone: var(--text-faint);
    color: var(--text-dim);
  }
  .dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--tone);
    flex-shrink: 0;
  }
  .meta {
    font-size: 0.75rem;
    color: var(--text-faint);
  }
</style>
