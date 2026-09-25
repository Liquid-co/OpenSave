<script>
  // Two snapshots of a game side by side: which files one has that the other
  // does not, and which both have in different versions. Read from the
  // archives' own lists, so it is instant (see snapshot/compare.go).
  import FilePen from 'lucide-svelte/icons/file-pen';
  import FilePlus from 'lucide-svelte/icons/file-plus';
  import FileMinus from 'lucide-svelte/icons/file-minus';
  import GitCompareArrows from 'lucide-svelte/icons/git-compare-arrows';
  import Modal from '../../components/ui/Modal.svelte';
  import Spinner from '../../components/ui/Spinner.svelte';
  import { api } from '../../lib/api.js';
  import { fmtSize } from '../../lib/format.js';
  import { snapshotKind, whenLabel } from '../../lib/snapshots.js';

  export let game;
  /** Every snapshot of the game, newest first. */
  export let snapshots = [];
  /** The one it was opened from: compared with the one before it. */
  export let start;
  export let onClose = () => {};

  const index = snapshots.findIndex((s) => s.id === start.id);
  let to = start.id;
  let from = (snapshots[index + 1] ?? snapshots[index - 1] ?? start).id;

  let result = null;
  let error = '';
  let loading = false;
  $: compare(from, to);
  async function compare(a, b) {
    loading = true;
    error = '';
    try {
      result = await api.get(`/api/games/${game.id}/snapshot/${a}/compare/${b}`);
    } catch (e) {
      error = e.message;
      result = null;
    } finally {
      loading = false;
    }
  }

  const label = (s) => `${whenLabel(s.timestamp)} — ${s.note || snapshotKind(s).title}`;
  const icons = { changed: FilePen, added: FilePlus, removed: FileMinus };
  const sizes = (c) =>
    c.change === 'changed' ? `${fmtSize(c.fromSize)} → ${fmtSize(c.toSize)}` : fmtSize(c.change === 'added' ? c.toSize : c.fromSize);
</script>

<Modal title="Compare snapshots" icon={GitCompareArrows} {onClose} width={640} height="auto" maxHeight="min(80vh, 680px)">
  <svelte:fragment slot="sub">{game.name}</svelte:fragment>
  <div class="body">
    <div class="pick">
      <label class="field">
        <span>From</span>
        <select bind:value={from}>
          {#each snapshots as s (s.id)}<option value={s.id}>{label(s)}</option>{/each}
        </select>
      </label>
      <label class="field">
        <span>To</span>
        <select bind:value={to}>
          {#each snapshots as s (s.id)}<option value={s.id}>{label(s)}</option>{/each}
        </select>
      </label>
    </div>

    {#if error}
      <p class="quiet">Couldn't compare them: {error}</p>
    {:else if !result}
      <p class="quiet"><Spinner size={13} /> Comparing…</p>
    {:else if result.changes.length === 0}
      <p class="quiet">{from === to ? 'The same snapshot.' : `No difference: the same ${result.unchanged} file${result.unchanged === 1 ? '' : 's'} in both.`}</p>
    {:else}
      <p class="count" class:stale={loading}>
        {result.changes.length} file{result.changes.length === 1 ? '' : 's'} differ{result.changes.length === 1 ? 's' : ''}{#if result.unchanged}, {result.unchanged} the same{/if}
      </p>
      <ul class:stale={loading}>
        {#each result.changes as c (c.location + '/' + c.path)}
          <li class="change {c.change}">
            <svelte:component this={icons[c.change]} size={14} />
            <span class="path" title={c.path}>{c.location ? `${c.location}/` : ''}{c.path}</span>
            <span class="kind">{c.change}</span>
            <span class="sizes">{sizes(c)}</span>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</Modal>

<style>
  .body {
    padding: 4px 20px 20px;
    overflow: auto;
  }
  .pick {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
    margin-bottom: 14px;
  }
  .pick label {
    display: flex;
    flex-direction: column;
    gap: 5px;
    margin: 0;
    font-size: 0.78rem;
    color: var(--text-faint);
    min-width: 0;
  }
  .pick select {
    min-width: 0;
  }
  .quiet,
  .count {
    font-size: 0.84rem;
    color: var(--text-dim);
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .count {
    margin-bottom: 8px;
  }
  ul {
    list-style: none;
    margin: 0;
    padding: 0;
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }
  .stale {
    opacity: 0.55;
  }
  .change {
    display: flex;
    align-items: center;
    gap: 9px;
    padding: 7px 12px;
    font-size: 0.84rem;
    border-top: 1px solid var(--border);
  }
  .change:first-child {
    border-top: none;
  }
  .change :global(svg) {
    flex-shrink: 0;
    color: var(--text-faint);
  }
  .change.added :global(svg) {
    color: var(--success);
  }
  .change.removed :global(svg) {
    color: var(--danger);
  }
  .change.changed :global(svg) {
    color: var(--accent);
  }
  .path {
    flex: 1;
    min-width: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .kind {
    font-size: 0.74rem;
    color: var(--text-faint);
    width: 58px;
  }
  .sizes {
    font-size: 0.76rem;
    color: var(--text-faint);
    white-space: nowrap;
    font-variant-numeric: tabular-nums;
  }
</style>
