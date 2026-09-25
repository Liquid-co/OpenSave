<script>
  // Everything that happened to the saves, game by game: where each game was
  // last played and when it was last snapshotted and synced, and then the
  // timeline itself — syncs with your other devices, snapshots, play
  // sessions, restores. See lib/timeline.js and internal/daemon/activity.go.
  import { onDestroy } from 'svelte';
  import ArrowDownToLine from 'lucide-svelte/icons/arrow-down-to-line';
  import ArrowUpFromLine from 'lucide-svelte/icons/arrow-up-from-line';
  import Trash2 from 'lucide-svelte/icons/trash-2';
  import RotateCcw from 'lucide-svelte/icons/rotate-ccw';
  import CloudDownload from 'lucide-svelte/icons/cloud-download';
  import TriangleAlert from 'lucide-svelte/icons/triangle-alert';
  import Camera from 'lucide-svelte/icons/camera';
  import Gamepad2 from 'lucide-svelte/icons/gamepad-2';
  import Dot from 'lucide-svelte/icons/dot';
  import CoverImage from '../../components/CoverImage.svelte';
  import FilterTabs from '../../components/ui/FilterTabs.svelte';
  import { api, gameCover } from '../../lib/api.js';
  import { games, navigate, activityTick } from '../../lib/stores.js';
  import { timeAgo } from '../../lib/timeago.js';
  import { playLength } from '../../lib/format.js';
  import { TIMELINE_FILTERS, byDay, filterItems, playedWhere, runs } from '../../lib/timeline.js';

  const ICONS = {
    down: ArrowDownToLine,
    up: ArrowUpFromLine,
    trash: Trash2,
    restore: RotateCcw,
    cloud: CloudDownload,
    alert: TriangleAlert,
    camera: Camera,
    play: Gamepad2,
    dot: Dot
  };
  const initials = (name = '') => name.split(/\s+/).map((w) => w[0]).join('').slice(0, 2).toUpperCase();
  const iso = (ms) => new Date(ms).toISOString();

  let report = null;
  let error = '';
  let loadingMore = false;
  let filter = 'all';
  let gameId = '';

  async function load() {
    try {
      report = await api.get('/api/activity?limit=150');
      error = '';
    } catch (e) {
      error = e.message;
    }
  }
  async function loadMore() {
    const oldest = report.items[report.items.length - 1]?.atMs;
    if (!oldest) return;
    loadingMore = true;
    try {
      const older = await api.get(`/api/activity?limit=150&before=${oldest}`);
      report = { ...report, items: [...report.items, ...older.items], more: older.more };
    } catch (e) {
      error = e.message;
    } finally {
      loadingMore = false;
    }
  }
  load();

  // New activity comes as it happens; gathered for a moment, since a sync
  // can bring several at once.
  let pending = null;
  const stop = activityTick.subscribe((n) => {
    if (!n) return;
    clearTimeout(pending);
    pending = setTimeout(load, 700);
  });
  let now = Date.now();
  const tick = setInterval(() => (now = Date.now()), 60_000);
  onDestroy(() => {
    stop();
    clearTimeout(pending);
    clearInterval(tick);
  });

  $: standing = (report?.games ?? [])
    .filter((g) => $games[g.gameId])
    .sort((a, b) => (b.lastPlayedAt || b.lastSnapshotAt || 0) - (a.lastPlayedAt || a.lastSnapshotAt || 0));
  $: shown = filterItems((report?.items ?? []).filter((it) => $games[it.gameId]), filter, gameId);
  $: days = byDay(runs(shown, new Date(now)), new Date(now));
</script>

{#if error}
  <div class="card"><p class="hint">Couldn't read the activity: {error}</p></div>
{:else if !report}
  <div class="skeleton" style="height: 220px"></div>
{:else}
  {#if standing.length}
    <div class="card standing">
      <div class="row header" aria-hidden="true">
        <span>Game</span><span>Last played</span><span>Last snapshot</span><span>Last synced</span><span>Played here</span>
      </div>
      {#each standing as g (g.gameId)}
        {@const game = $games[g.gameId]}
        <button class="row" on:click={() => navigate('game', { gameId: g.gameId })} title="Open {game.name}">
          <span class="game">
            <span class="thumb"><span class="initials">{initials(game.name)}</span><CoverImage src={gameCover(game)} alt="" /></span>
            <span class="name">{game.name}</span>
          </span>
          <span class="cell">
            {#if g.lastPlayedAt}
              <span class="main">{playedWhere(g, report.device)}</span>
              <span class="sub">{timeAgo(iso(g.lastPlayedAt), now)}</span>
            {:else}<span class="none">—</span>{/if}
          </span>
          <span class="cell">
            {#if g.lastSnapshotAt}<span class="main">{timeAgo(iso(g.lastSnapshotAt), now)}</span>{:else}<span class="none">None yet</span>{/if}
          </span>
          <span class="cell">
            {#if g.lastSyncedAt}
              <span class="main">{g.lastSyncedWith || 'Another device'}</span>
              <span class="sub">{timeAgo(iso(g.lastSyncedAt), now)}</span>
            {:else}<span class="none">Not yet</span>{/if}
          </span>
          <span class="cell">
            {#if g.playtimeMs}
              <span class="main">{playLength(g.playtimeMs)}</span>
              <span class="sub">{g.sessions} session{g.sessions === 1 ? '' : 's'}</span>
            {:else}<span class="none">—</span>{/if}
          </span>
        </button>
      {/each}
    </div>
  {/if}

  <div class="filters">
    <FilterTabs options={Object.entries(TIMELINE_FILTERS).map(([id, f]) => [id, f.label])} bind:value={filter} />
    <select bind:value={gameId} aria-label="Show one game">
      <option value="">Every game</option>
      {#each Object.values($games).sort((a, b) => a.name.localeCompare(b.name)) as game (game.id)}
        <option value={game.id}>{game.name}</option>
      {/each}
    </select>
  </div>

  {#if days.length === 0}
    <div class="card"><div class="empty"><h3>Nothing here yet</h3><p>Syncs, snapshots and play sessions show up here as they happen.</p></div></div>
  {:else}
    {#each days as day (day.key)}
      <h4 class="day">{day.day}</h4>
      <div class="card entries">
        {#each day.runs as run}
          {@const game = $games[run.item.gameId]}
          <button class="entry tone-{run.said.tone}" on:click={() => navigate('game', { gameId: run.item.gameId })}>
            <span class="kind"><svelte:component this={ICONS[run.said.icon] ?? Dot} size={15} /></span>
            <span class="thumb"><span class="initials">{initials(game.name)}</span><CoverImage src={gameCover(game)} alt="" /></span>
            <span class="what">
              <span class="line"><span class="gname">{game.name}</span> {run.said.title}{#if run.count > 1}<span class="count">×{run.count}</span>{/if}</span>
              {#if run.said.detail}<span class="sub">{run.said.detail}</span>{/if}
            </span>
            <span class="clock" title={new Date(run.item.atMs).toLocaleString()}>
              {new Date(run.item.atMs).toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })}
            </span>
          </button>
        {/each}
      </div>
    {/each}
    {#if report.more && filter === 'all' && !gameId}
      <div class="more">
        <button class="btn small" disabled={loadingMore} on:click={loadMore}>{loadingMore ? 'Loading…' : 'Show older'}</button>
      </div>
    {/if}
  {/if}
{/if}

<style>
  .standing {
    padding: 6px 8px;
    margin-bottom: 18px;
  }
  .row {
    display: grid;
    grid-template-columns: minmax(180px, 1.4fr) 1fr 0.8fr 1fr 0.8fr;
    gap: 12px;
    align-items: center;
    width: 100%;
    padding: 8px;
    border: none;
    border-radius: 8px;
    background: transparent;
    color: var(--text);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .row:not(.header):hover {
    background: var(--bg-hover);
  }
  .row.header {
    cursor: default;
    padding-top: 6px;
    padding-bottom: 4px;
    font-size: 0.72rem;
    font-weight: 600;
    letter-spacing: 0.03em;
    text-transform: uppercase;
    color: var(--text-faint);
  }
  .game {
    display: flex;
    align-items: center;
    gap: 10px;
    min-width: 0;
  }
  .name {
    font-weight: 600;
    font-size: 0.88rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .cell {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .main {
    font-size: 0.84rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .sub {
    font-size: 0.76rem;
    color: var(--text-faint);
  }
  .none {
    font-size: 0.82rem;
    color: var(--text-faint);
  }
  .thumb {
    position: relative;
    flex: none;
    width: 30px;
    height: 30px;
    border-radius: 6px;
    overflow: hidden;
    background: var(--bg-hover);
    display: grid;
    place-items: center;
  }
  .thumb :global(img) {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .initials {
    font-size: 0.66rem;
    font-weight: 700;
    color: var(--text-faint);
  }
  .filters {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    flex-wrap: wrap;
    margin-bottom: 10px;
  }
  select {
    padding: 6px 10px;
    background-color: var(--bg);
    border: 1px solid var(--border-strong);
    border-radius: 8px;
    color: var(--text);
    font-size: 0.85rem;
  }
  .day {
    margin: 16px 0 8px;
    font-size: 0.78rem;
    font-weight: 600;
    color: var(--text-faint);
  }
  .entries {
    padding: 4px;
  }
  .entry {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    padding: 7px 8px;
    border: none;
    border-radius: 8px;
    background: transparent;
    color: var(--text);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .entry:hover {
    background: var(--bg-hover);
  }
  .kind {
    flex: none;
    width: 18px;
    display: grid;
    place-items: center;
    color: var(--text-faint);
  }
  .tone-warn .kind {
    color: var(--warn);
  }
  .tone-ok .kind {
    color: var(--text-dim);
  }
  .what {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }
  .line {
    font-size: 0.86rem;
    color: var(--text-dim);
  }
  .gname {
    font-weight: 600;
    color: var(--text);
  }
  .count {
    margin-left: 6px;
    font-size: 0.76rem;
    color: var(--text-faint);
  }
  .clock {
    flex: none;
    font-size: 0.76rem;
    color: var(--text-faint);
  }
  .more {
    display: flex;
    justify-content: center;
    margin: 14px 0;
  }
  .empty {
    text-align: center;
    padding: 28px 12px;
  }
  .empty p {
    color: var(--text-faint);
    font-size: 0.86rem;
  }
</style>
