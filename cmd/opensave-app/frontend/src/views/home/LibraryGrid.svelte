<script>
  // The tracked games, as cover cards. "Select" turns the grid into a
  // multi-select, to act on several games at once (e.g. clear a batch of
  // wrongly-tracked entries) without going in and out of each one.
  import { onDestroy } from 'svelte';
  import { gameList, peers, navigate, toast, syncActivity, askConfirm } from '../../lib/stores.js';
  import { api, gameCover } from '../../lib/api.js';
  import { timeAgo, latestOf } from '../../lib/timeago.js';

  // The card gets one line per game — the most recent paired device to
  // confirm this save; the game's own page lists every device. Only stamps
  // from devices still paired count, and only when there is a device to be
  // up to date with: a library with nothing paired has nothing to say here.
  $: hasPeers = Object.keys($peers).length > 0;
  // The peer map is passed in rather than read from the store inside, so the
  // template re-evaluates when a device is unpaired, not only when a game
  // changes.
  const lastSyncedWithPaired = (game, paired) =>
    Object.fromEntries(Object.entries(game.lastSyncedWith ?? {}).filter(([id]) => id in paired));
  // "4 min ago" must not freeze at the moment the shelf was opened.
  let now = Date.now();
  const tick = setInterval(() => (now = Date.now()), 30_000);
  onDestroy(() => clearInterval(tick));

  let selectMode = false;
  let libSelected = new Set();
  let libSelectedCount = 0;

  function toggleSelectMode() {
    selectMode = !selectMode;
    if (!selectMode) clearSelection();
  }
  function toggleSelect(id) {
    if (libSelected.has(id)) libSelected.delete(id);
    else libSelected.add(id);
    libSelected = libSelected;
    libSelectedCount = libSelected.size;
  }
  function selectAll() {
    for (const g of $gameList) libSelected.add(g.id);
    libSelected = libSelected;
    libSelectedCount = libSelected.size;
  }
  function clearSelection() {
    libSelected = new Set();
    libSelectedCount = 0;
  }
  $: allSelected = $gameList.length > 0 && libSelectedCount === $gameList.length;
  function toggleSelectAll() {
    if (allSelected) clearSelection();
    else selectAll();
  }
  async function untrackSelected() {
    const n = libSelectedCount;
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
      clearSelection();
    }
  }

  const snapshotCount = (game) =>
    Object.values(game.branches ?? {}).reduce((n, b) => n + (b.snapshots?.length ?? 0), 0);
</script>

<div class="section-row">
  <h3>Library</h3>
  {#if selectMode}
    <div class="select-bar">
      <span class="select-count">{libSelectedCount} selected</span>
      <button class="btn small" on:click={toggleSelectAll}>
        {allSelected ? 'Unselect all' : `Select all (${$gameList.length})`}
      </button>
      <button class="btn small danger" disabled={libSelectedCount === 0} on:click={untrackSelected}>
        Untrack selected
      </button>
      <button class="btn small" on:click={toggleSelectMode}>Cancel</button>
    </div>
  {:else}
    <button class="btn small" on:click={toggleSelectMode}>☑ Select</button>
  {/if}
</div>
<div class="grid">
  {#each $gameList as game (game.id)}
    <button
      class="card game-card"
      class:selected={selectMode && libSelected.has(game.id)}
      on:click={() => (selectMode ? toggleSelect(game.id) : navigate('game', { gameId: game.id }))}
    >
      {#if selectMode}
        <div class="tick" class:on={libSelected.has(game.id)}>{libSelected.has(game.id) ? '✓' : ''}</div>
      {/if}
      <div class="cover">
        {#if gameCover(game)}
          <img
            src={gameCover(game)}
            alt=""
            loading="lazy"
            on:load={(e) => (e.currentTarget.style.display = '')}
            on:error={(e) => (e.currentTarget.style.display = 'none')}
          />
        {/if}
        <div class="cover-fallback"><span>{game.name}</span></div>
      </div>
      <div class="body">
        <div class="name">{game.name}</div>
        <div class="meta">
          branch <strong>{game.activeBranch}</strong>
          · {snapshotCount(game)} snapshots
          {#if hasPeers}
            {@const at = latestOf(lastSyncedWithPaired(game, $peers))}
            · <span class="synced" class:never={!at} title={at ? 'Most recent device — open the game to see each one' : 'No paired device has synced this game yet'}>
              {at ? `synced ${timeAgo(at, now)}` : 'never synced'}
            </span>
          {/if}
        </div>
        <div class="path" title={game.savePath}>{game.savePath}</div>
        {#if $syncActivity[game.id]?.state === 'running'}
          <div class="sync">syncing… {$syncActivity[game.id].percentage ?? 0}%</div>
        {/if}
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
  .select-bar {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }
  .select-count {
    color: var(--text-dim);
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
  }
  .cover img {
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
    padding: 14px 16px;
  }
  .name {
    font-weight: 600;
    margin-bottom: 6px;
  }
  .meta {
    font-size: 0.8rem;
    color: var(--text-dim);
    margin-bottom: 6px;
  }
  .path {
    font-size: 0.73rem;
    color: var(--text-faint);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .sync {
    margin-top: 8px;
    font-size: 0.78rem;
    color: var(--accent);
    font-weight: 600;
  }
  .synced.never {
    font-style: italic;
  }
</style>
