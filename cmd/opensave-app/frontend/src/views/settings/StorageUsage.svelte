<script>
  // Where the space goes: each game's share, the biggest snapshots, free
  // space, and what clean-up would give back — before pressing it.
  import { onMount } from 'svelte';
  import BrushCleaning from 'lucide-svelte/icons/brush-cleaning';
  import Pin from 'lucide-svelte/icons/pin';
  import Trash2 from 'lucide-svelte/icons/trash-2';
  import { api } from '../../lib/api.js';
  import { askConfirm, navigate, toast } from '../../lib/stores.js';
  import { fmtSize } from '../../lib/format.js';
  import { whenLabel } from '../../lib/snapshots.js';

  let report = null;
  let error = '';
  let busy = false;

  async function load() {
    try {
      report = await api.get('/api/storage');
      error = '';
    } catch (e) {
      error = e.message;
    }
  }
  onMount(load);

  $: largest = Math.max(1, ...(report?.games ?? []).map((g) => g.bytes));

  async function cleanUp() {
    busy = true;
    try {
      const res = await api.post('/api/snapshots/prune', {});
      toast(res.removed > 0 ? `Removed ${res.removed} snapshot(s), freed ${fmtSize(res.freedBytes)}` : 'Nothing to clean up', 'success');
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      busy = false;
      load();
    }
  }

  async function remove(s) {
    const pinned = s.pinned ? ' It is pinned, so nothing else would ever have removed it.' : '';
    if (!(await askConfirm(`Delete the ${fmtSize(s.bytes)} snapshot of ${s.gameName} from ${whenLabel(s.timestamp)}?${pinned} This can't be undone; the current save isn't affected.`, { title: 'Delete snapshot?', confirmText: 'Delete', danger: true }))) return;
    try {
      await api.del(`/api/games/${s.gameId}/snapshot/${s.snapshotId}`);
      toast(`Freed ${fmtSize(s.bytes)}`, 'success');
    } catch (e) {
      toast(e.message, 'error');
    }
    load();
  }
</script>

{#if error}
  <p class="hint">Couldn't read storage: {error}</p>
{:else if !report}
  <div class="skeleton" style="height: 120px"></div>
{:else}
  <div class="summary">
    <div>
      <div class="total">{fmtSize(report.totalBytes)}</div>
      <div class="hint">in {report.snapshots} snapshot{report.snapshots === 1 ? '' : 's'}{#if report.freeKnown}{' · '}{fmtSize(report.freeBytes)} free on that drive{/if}</div>
    </div>
    <button class="btn small" class:primary={report.reclaimable > 0} disabled={busy || report.reclaimable === 0} on:click={cleanUp}>
      <BrushCleaning size={14} />
      {#if busy}Cleaning up…{:else if report.reclaimable > 0}Clean up now · frees {fmtSize(report.reclaimable)}{:else}Nothing to clean up{/if}
    </button>
  </div>
  {#if report.reclaimable > 0}
    <p class="hint">
      {report.reclaimableSnapshots} snapshot{report.reclaimableSnapshots === 1 ? ' is' : 's are'} past a game's limits, on an old
      conflict branch, or past the age rule. Pinned snapshots are never among them.
    </p>
  {/if}

  {#if report.games.length}
    <ul class="games">
      {#each report.games as g}
        <li>
          <button class="game" on:click={() => navigate('game', { gameId: g.gameId })} title="Open {g.name}">
            <span class="name">{g.name}</span>
            <span class="bar"><span style="width: {Math.max(2, (g.bytes / largest) * 100)}%"></span></span>
            <span class="size">{fmtSize(g.bytes)}</span>
            <span class="meta">
              {g.snapshots} snapshot{g.snapshots === 1 ? '' : 's'}{#if g.pinned}{' · '}{g.pinned} pinned{/if}{#if g.reclaimable}{' · '}{fmtSize(g.reclaimable)} to clean up{/if}
            </span>
          </button>
        </li>
      {/each}
    </ul>
  {/if}

  {#if report.biggest.length}
    <h4>Biggest snapshots</h4>
    <ul class="biggest">
      {#each report.biggest as s}
        <li>
          <span class="size">{fmtSize(s.bytes)}</span>
          <span class="what">
            <span class="name">{s.gameName}</span>
            <span class="meta">{whenLabel(s.timestamp)}{#if s.note}{' · '}{s.note}{/if}</span>
          </span>
          {#if s.pinned}<span class="pinned"><Pin size={11} />Pinned</span>{/if}
          <button class="btn small ghost icon" title="Delete this snapshot" aria-label="Delete snapshot" on:click={() => remove(s)}><Trash2 size={14} /></button>
        </li>
      {/each}
    </ul>
  {/if}
{/if}

<style>
  .summary {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    margin-bottom: 8px;
  }
  .total {
    font-size: 1.5rem;
    font-weight: 700;
    letter-spacing: -0.01em;
  }
  .games,
  .biggest {
    list-style: none;
    margin-top: 12px;
  }
  .game {
    display: grid;
    grid-template-columns: minmax(120px, 200px) 1fr 80px;
    grid-template-rows: auto auto;
    column-gap: 12px;
    align-items: center;
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
  .game:hover {
    background: var(--bg-hover);
  }
  .game .name {
    font-size: 0.88rem;
    font-weight: 600;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .bar {
    height: 8px;
    border-radius: 999px;
    background: var(--bg-active);
    overflow: hidden;
  }
  .bar span {
    display: block;
    height: 100%;
    border-radius: 999px;
    background: var(--accent);
  }
  .size {
    font-size: 0.84rem;
    font-variant-numeric: tabular-nums;
    text-align: right;
    white-space: nowrap;
  }
  .game .meta {
    grid-column: 1 / -1;
    font-size: 0.74rem;
    color: var(--text-faint);
    margin-top: 2px;
  }
  h4 {
    margin: 18px 0 0;
    font-size: 0.72rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-faint);
  }
  .biggest li {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 6px 8px;
    border-radius: 8px;
  }
  .biggest li:hover {
    background: var(--bg-hover);
  }
  .biggest .size {
    width: 70px;
    text-align: left;
    font-weight: 600;
  }
  .what {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }
  .what .name {
    font-size: 0.86rem;
  }
  .what .meta {
    font-size: 0.74rem;
    color: var(--text-faint);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .pinned {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    padding: 0 7px;
    border-radius: 999px;
    background: var(--accent-soft);
    color: var(--accent);
    font-size: 0.7rem;
    font-weight: 600;
  }
</style>
