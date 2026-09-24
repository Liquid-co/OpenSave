<script>
  // "Restore this snapshot?", answered knowing what it would do: the files it
  // changes, brings back and removes, worked out by the daemon without
  // touching anything. See lib/restore.js.
  import RotateCcw from 'lucide-svelte/icons/rotate-ccw';
  import FilePen from 'lucide-svelte/icons/file-pen';
  import FilePlus from 'lucide-svelte/icons/file-plus';
  import FileMinus from 'lucide-svelte/icons/file-minus';
  import ShieldCheck from 'lucide-svelte/icons/shield-check';
  import Modal from './ui/Modal.svelte';
  import ModalFoot from './ui/ModalFoot.svelte';
  import Spinner from './ui/Spinner.svelte';
  import { api } from '../lib/api.js';
  import { fmtSize } from '../lib/format.js';
  import { whenLabel } from '../lib/snapshots.js';
  import { restoreRequest, answerRestore, summarize, describeSummary } from '../lib/restore.js';

  let preview = null;
  let error = '';
  let loadedFor = null;

  $: req = $restoreRequest;
  $: if (req && loadedFor !== req.snap.id) load(req);

  async function load(r) {
    loadedFor = r.snap.id;
    preview = null;
    error = '';
    try {
      preview = summarize(await api.get(`/api/games/${r.game.id}/snapshot/${r.snap.id}/preview`));
    } catch (e) {
      // Not knowing is no reason to refuse: the restore is still safe to do,
      // and the dialog says it could not look first.
      error = e.message;
    }
  }

  function answer(yes) {
    loadedFor = null;
    answerRestore(yes);
  }

  const icons = { changed: FilePen, restored: FilePlus, removed: FileMinus };
  const sizes = (c) =>
    c.change === 'changed'
      ? `${fmtSize(c.currentSize)} → ${fmtSize(c.snapshotSize)}`
      : fmtSize(c.change === 'restored' ? c.snapshotSize : c.currentSize);
</script>

{#if req}
  <Modal title="Restore this snapshot?" icon={RotateCcw} onClose={() => answer(false)} width={600} height="auto" maxHeight="min(80vh, 680px)">
    <svelte:fragment slot="sub">
      {req.game.name} · the snapshot from {whenLabel(req.snap.timestamp)}
    </svelte:fragment>

    <div class="body">
      {#if !preview && !error}
        <p class="quiet"><Spinner size={13} /> Comparing it with your save…</p>
      {:else if error}
        <p class="quiet">Couldn't compare it with your save first ({error}). Restoring is still safe.</p>
      {:else if preview.identical}
        <p class="lead">Your save already matches this snapshot — restoring it would change nothing.</p>
      {:else}
        <p class="lead">{describeSummary(preview)}{#if preview.unchanged}; {preview.unchanged} {preview.unchanged === 1 ? 'stays' : 'stay'} as {preview.unchanged === 1 ? 'it is' : 'they are'}{/if}.</p>
        <ul class="files">
          {#each preview.changes as c}
            <li class="file {c.change}">
              <span class="kind"><svelte:component this={icons[c.change]} size={14} /></span>
              <span class="path" title={c.path}>{#if c.location}<span class="loc">{c.location}</span>{/if}{c.path}</span>
              <span class="what">{c.change === 'changed' ? 'changes' : c.change === 'restored' ? 'comes back' : 'removed'}</span>
              <span class="size">{sizes(c)}</span>
            </li>
          {/each}
        </ul>
      {/if}
      {#if preview?.unplaced?.length}
        <p class="quiet">
          It also has files for {preview.unplaced.map((n) => `“${n}”`).join(', ')}, which has no folder on this
          device; they are left out.
        </p>
      {/if}
      <p class="safe"><ShieldCheck size={14} class="inline-icon" />Your current save is kept as a snapshot first, so this can be undone.</p>
    </div>

    <ModalFoot>
      <span></span>
      <div class="actions">
        <button class="btn" on:click={() => answer(false)}>Cancel</button>
        <button class="btn primary" on:click={() => answer(true)}>{preview?.identical ? 'Restore anyway' : 'Restore'}</button>
      </div>
    </ModalFoot>
  </Modal>
{/if}

<style>
  .body {
    padding: 16px 22px 6px;
    display: flex;
    flex-direction: column;
    gap: 12px;
    min-height: 0;
    overflow: auto;
  }
  .lead {
    font-size: 0.92rem;
  }
  .quiet {
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--text-dim);
    font-size: 0.86rem;
  }
  .files {
    list-style: none;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    max-height: 320px;
    overflow: auto;
  }
  .file {
    display: grid;
    grid-template-columns: 18px minmax(0, 1fr) auto auto;
    align-items: center;
    gap: 10px;
    padding: 7px 12px;
    font-size: 0.85rem;
    border-top: 1px solid var(--border);
  }
  .file:first-child {
    border-top: none;
  }
  .kind {
    display: flex;
  }
  .file.changed .kind {
    color: var(--accent);
  }
  .file.restored .kind {
    color: var(--success);
  }
  .file.removed .kind {
    color: var(--warn);
  }
  .path {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    font-family: ui-monospace, 'Cascadia Code', Consolas, monospace;
    font-size: 0.8rem;
  }
  .loc {
    margin-right: 6px;
    padding: 0 6px;
    border-radius: 999px;
    background: var(--btn-bg);
    color: var(--text-dim);
    font-family: inherit;
  }
  .what,
  .size {
    color: var(--text-faint);
    font-size: 0.76rem;
    white-space: nowrap;
  }
  .safe {
    font-size: 0.82rem;
    color: var(--text-dim);
  }
  .actions {
    display: flex;
    gap: 8px;
  }
</style>
