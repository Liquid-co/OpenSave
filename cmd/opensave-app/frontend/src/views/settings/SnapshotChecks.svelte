<script>
  // Whether every snapshot can still be restored: when they were last read
  // back, what was found, and a check now. See daemon/verify.go.
  import { onMount } from 'svelte';
  import ShieldCheck from 'lucide-svelte/icons/shield-check';
  import TriangleAlert from 'lucide-svelte/icons/triangle-alert';
  import { api } from '../../lib/api.js';
  import { games, navigate, toast } from '../../lib/stores.js';
  import { timeAgo } from '../../lib/timeago.js';
  import { whenLabel } from '../../lib/snapshots.js';

  let summary = null;
  let checking = false;

  async function load() {
    try {
      summary = await api.get('/api/snapshots/check');
    } catch {
      summary = null;
    }
  }
  onMount(load);

  async function checkNow() {
    checking = true;
    try {
      const report = await api.post('/api/snapshots/check', {});
      summary = report.summary;
      toast(
        report.damaged.length
          ? `${report.damaged.length} of ${report.checked} snapshots can't be restored`
          : `Checked ${report.checked} snapshots — every one can be restored`,
        report.damaged.length ? 'error' : 'success'
      );
    } catch (e) {
      toast(e.message, 'error');
    } finally {
      checking = false;
    }
  }

  const iso = (ms) => new Date(ms).toISOString();
  const nameOf = (id) => $games[id]?.name ?? id;
</script>

<div class="checks">
  {#if summary}
    <div class="state" class:bad={summary.damaged.length}>
      {#if summary.damaged.length}
        <TriangleAlert size={18} />
        <span>
          <strong>{summary.damaged.length} {summary.damaged.length === 1 ? "snapshot can't" : "snapshots can't"} be restored.</strong>
          Their archives are damaged or missing; the rest are fine.
        </span>
      {:else if summary.lastCheckedMs}
        <ShieldCheck size={18} />
        <span>
          <strong>Every snapshot checked can be restored.</strong>
          {summary.checked} of {summary.total} checked, last {timeAgo(iso(summary.lastCheckedMs))}.
        </span>
      {:else}
        <ShieldCheck size={18} />
        <span>Not checked yet — OpenSave reads every snapshot back once a day, the first time a little after it starts.</span>
      {/if}
    </div>
    {#if summary.damaged.length}
      <ul class="damaged">
        {#each summary.damaged as s (s.id)}
          <li>
            <button class="linklike" on:click={() => navigate('game', { gameId: s.gameId, snapshot: s.id })}>
              {nameOf(s.gameId)} — {whenLabel(s.timestamp)}
            </button>
            <span class="why">{s.problem}</span>
          </li>
        {/each}
      </ul>
    {/if}
  {/if}
  <div class="actions">
    <button class="btn small" disabled={checking} on:click={checkNow}>{checking ? 'Checking…' : 'Check every snapshot now'}</button>
    <span class="hint">Each is read back in full and every file compared with its checksum. A snapshot is also checked before it is restored.</span>
  </div>
</div>

<style>
  .checks {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .state {
    display: flex;
    gap: 10px;
    align-items: flex-start;
    font-size: 0.86rem;
    color: var(--text-dim);
  }
  .state :global(svg) {
    flex-shrink: 0;
    margin-top: 1px;
    color: var(--success);
  }
  .state.bad :global(svg) {
    color: var(--danger);
  }
  .state strong {
    color: var(--text);
  }
  .damaged {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
    font-size: 0.82rem;
  }
  .damaged li {
    display: flex;
    flex-direction: column;
    gap: 1px;
  }
  .why {
    color: var(--danger-text);
    font-size: 0.78rem;
  }
  .linklike {
    background: none;
    border: none;
    padding: 0;
    font: inherit;
    color: var(--text);
    text-align: left;
    cursor: pointer;
    text-decoration: underline;
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 10px;
  }
  .actions .hint {
    margin: 0;
  }
</style>
